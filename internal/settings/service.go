// Package settings 解析与变更“管理画面可配置、重启后保留”的运行时设置。
//
// 持久化只存“覆盖值”（domain.SettingsRepository）；有效值按
// DB 覆盖 > env > 代码默认值 的优先级解析（docs/architecture.md §9.1）。
// 同步引擎在每次运行前调用 ResolveSyncSettings 取有效配置，从而让管理画面的
// 改动无需重启即可在 batch / 手动触发 / 上传等路径生效。
//
// 输入校验（URL 的 SSRF 防护、重试次数范围）在写入前完成，错误文案为日语，
// 供管理画面直接展示。
package settings

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// MaxDownloadRetry 是 download_max_retry 的上限，避免管理画面配出会拖垮同步的
// 超大重试次数。下限为 0（不重试）。
const MaxDownloadRetry = 10

// allowedScrapeHosts 是全量抓取 URL 允许的主机白名单（SSRF 防护）：仅日本邮便
// 官方域名。新增官方子域时在此扩充。
var allowedScrapeHosts = map[string]bool{
	"www.post.japanpost.jp": true,
	"post.japanpost.jp":     true,
}

// Defaults 是未被覆盖时的基线值（env 优先，缺省落到代码常量；由 config 注入）。
// “恢复默认”即把有效值回退到这里。
type Defaults struct {
	ScrapeFullURL    string
	DownloadMaxRetry int
	TownSkipRegex    string
}

// Service 解析与变更运行时设置。并发安全由底层 repo / DB 保证；同步引擎与
// 管理 API 共享同一实例。
type Service struct {
	repo     domain.SettingsRepository
	defaults Defaults
	now      func() time.Time
}

// NewService 构造服务。now 可注入便于测试；为 nil 时用 time.Now。
func NewService(repo domain.SettingsRepository, defaults Defaults, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, defaults: defaults, now: now}
}

// ValidationError 表示输入校验失败，Field 指出出错字段，Message 为日语提示。
// 管理 handler 据此返回 400 invalid_request 并回显 Message。
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Resolved 是一次解析得到的完整视图：每个键的有效值、默认值与是否被覆盖。
// 管理 API 的 GET/PUT 返回它，前端据 Overridden 决定是否显示“恢复默认”。
type Resolved struct {
	DownloadMaxRetry        int
	DownloadMaxRetryDefault int
	DownloadMaxRetryOver    bool
	ScrapeFullURL           string
	ScrapeFullURLDefault    string
	ScrapeFullURLOver       bool
	TownSkipRegex           string
	TownSkipRegexDefault    string
	TownSkipRegexOver       bool
}

// ResolveSyncSettings 返回同步引擎所需的有效配置（DB 覆盖 > env > 默认）。
// 存储中的覆盖值理应已在写入时校验过；为健壮起见这里对解析失败的值做防御式回退，
// 避免脏数据使同步整体失败。
func (s *Service) ResolveSyncSettings(ctx context.Context) (domain.EffectiveSyncSettings, error) {
	r, err := s.resolve(ctx)
	if err != nil {
		return domain.EffectiveSyncSettings{}, err
	}
	return domain.EffectiveSyncSettings{
		ScrapeFullURL:    r.ScrapeFullURL,
		DownloadMaxRetry: r.DownloadMaxRetry,
		TownSkipRegex:    r.TownSkipRegex,
	}, nil
}

// Get 返回当前完整视图，供管理 API 的 GET /v1/admin/settings 使用。
func (s *Service) Get(ctx context.Context) (Resolved, error) {
	return s.resolve(ctx)
}

func (s *Service) resolve(ctx context.Context) (Resolved, error) {
	overrides, err := s.repo.GetAll(ctx)
	if err != nil {
		return Resolved{}, err
	}
	out := Resolved{
		DownloadMaxRetry:        s.defaults.DownloadMaxRetry,
		DownloadMaxRetryDefault: s.defaults.DownloadMaxRetry,
		ScrapeFullURL:           s.defaults.ScrapeFullURL,
		ScrapeFullURLDefault:    s.defaults.ScrapeFullURL,
		TownSkipRegex:           s.defaults.TownSkipRegex,
		TownSkipRegexDefault:    s.defaults.TownSkipRegex,
	}
	if raw, ok := overrides[domain.SettingDownloadMaxRetry]; ok {
		if n, perr := strconv.Atoi(strings.TrimSpace(raw)); perr == nil && validateRetry(n) == nil {
			out.DownloadMaxRetry = n
			out.DownloadMaxRetryOver = true
		}
	}
	if raw, ok := overrides[domain.SettingScrapeFullURL]; ok {
		if validateScrapeURL(raw) == nil {
			out.ScrapeFullURL = strings.TrimSpace(raw)
			out.ScrapeFullURLOver = true
		}
	}
	if raw, ok := overrides[domain.SettingTownSkipRegex]; ok {
		if validateTownSkipRegex(raw) == nil {
			out.TownSkipRegex = strings.TrimSpace(raw)
			out.TownSkipRegexOver = true
		}
	}
	return out, nil
}

// UpdateInput 是 PUT /v1/admin/settings 的归一化入参。指针字段为 nil 表示“不变”；
// ResetToDefault 列出的键删除其覆盖值（恢复默认），且不得与同键的设值并存。
type UpdateInput struct {
	DownloadMaxRetry *int
	ScrapeFullURL    *string
	TownSkipRegex    *string
	ResetToDefault   []domain.RuntimeSettingKey
}

// Update 校验并应用一次设置变更，返回应用后的完整视图。任一字段校验失败时不写入
// 任何变更（先校验后写入），返回 *ValidationError。
func (s *Service) Update(ctx context.Context, in UpdateInput) (Resolved, error) {
	reset := map[domain.RuntimeSettingKey]bool{}
	for _, k := range in.ResetToDefault {
		switch k {
		case domain.SettingDownloadMaxRetry, domain.SettingScrapeFullURL, domain.SettingTownSkipRegex:
			reset[k] = true
		default:
			return Resolved{}, &ValidationError{Field: string(k), Message: "未知の設定キーです。"}
		}
	}

	// 设值与“恢复默认”互斥，避免歧义。
	if in.DownloadMaxRetry != nil && reset[domain.SettingDownloadMaxRetry] {
		return Resolved{}, &ValidationError{Field: string(domain.SettingDownloadMaxRetry), Message: "同じ項目を更新と既定値リセットの両方に指定できません。"}
	}
	if in.ScrapeFullURL != nil && reset[domain.SettingScrapeFullURL] {
		return Resolved{}, &ValidationError{Field: string(domain.SettingScrapeFullURL), Message: "同じ項目を更新と既定値リセットの両方に指定できません。"}
	}
	if in.TownSkipRegex != nil && reset[domain.SettingTownSkipRegex] {
		return Resolved{}, &ValidationError{Field: string(domain.SettingTownSkipRegex), Message: "同じ項目を更新と既定値リセットの両方に指定できません。"}
	}

	// 先全部校验，再落库——避免部分写入。
	if in.DownloadMaxRetry != nil {
		if err := validateRetry(*in.DownloadMaxRetry); err != nil {
			return Resolved{}, err
		}
	}
	var normalizedURL string
	if in.ScrapeFullURL != nil {
		if err := validateScrapeURL(*in.ScrapeFullURL); err != nil {
			return Resolved{}, err
		}
		normalizedURL = strings.TrimSpace(*in.ScrapeFullURL)
	}
	var normalizedTownSkipRegex string
	if in.TownSkipRegex != nil {
		if err := validateTownSkipRegex(*in.TownSkipRegex); err != nil {
			return Resolved{}, err
		}
		normalizedTownSkipRegex = strings.TrimSpace(*in.TownSkipRegex)
	}

	if in.DownloadMaxRetry != nil {
		if err := s.repo.Set(ctx, domain.SettingDownloadMaxRetry, strconv.Itoa(*in.DownloadMaxRetry)); err != nil {
			return Resolved{}, err
		}
	}
	if in.ScrapeFullURL != nil {
		if err := s.repo.Set(ctx, domain.SettingScrapeFullURL, normalizedURL); err != nil {
			return Resolved{}, err
		}
	}
	if in.TownSkipRegex != nil {
		if normalizedTownSkipRegex == "" {
			if err := s.repo.Delete(ctx, domain.SettingTownSkipRegex); err != nil {
				return Resolved{}, err
			}
		} else if err := s.repo.Set(ctx, domain.SettingTownSkipRegex, normalizedTownSkipRegex); err != nil {
			return Resolved{}, err
		}
	}
	for k := range reset {
		if err := s.repo.Delete(ctx, k); err != nil {
			return Resolved{}, err
		}
	}
	return s.resolve(ctx)
}

// validateRetry 校验重试次数范围。
func validateRetry(n int) error {
	if n < 0 || n > MaxDownloadRetry {
		return &ValidationError{
			Field:   string(domain.SettingDownloadMaxRetry),
			Message: "リトライ回数は 0 以上 10 以下の整数で指定してください。",
		}
	}
	return nil
}

// validateScrapeURL 对全量抓取 URL 做 SSRF 防护校验：必须是 https、主机在日本邮便
// 官方域名白名单内、且不含用户名信息。错误文案为日语。
func validateScrapeURL(raw string) error {
	field := string(domain.SettingScrapeFullURL)
	v := strings.TrimSpace(raw)
	if v == "" {
		return &ValidationError{Field: field, Message: "URL を入力してください。"}
	}
	u, err := url.Parse(v)
	if err != nil {
		return &ValidationError{Field: field, Message: "URL の形式が正しくありません。"}
	}
	if u.Scheme != "https" {
		return &ValidationError{Field: field, Message: "URL は https で指定してください。"}
	}
	if u.User != nil {
		return &ValidationError{Field: field, Message: "URL にユーザー情報を含めることはできません。"}
	}
	host := strings.ToLower(u.Hostname())
	if !allowedScrapeHosts[host] {
		return &ValidationError{Field: field, Message: "URL のドメインは日本郵便の公式サイト（post.japanpost.jp）のみ許可されています。"}
	}
	return nil
}

func validateTownSkipRegex(raw string) error {
	field := string(domain.SettingTownSkipRegex)
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	if _, err := regexp.Compile(v); err != nil {
		return &ValidationError{Field: field, Message: "町域名フィルターの正規表現が正しくありません。"}
	}
	return nil
}

// IsValidation 判断 err 是否为输入校验错误。
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
