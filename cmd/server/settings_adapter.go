package main

import (
	"context"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/server"
	"github.com/r404r/JapanDigitalPostService/internal/settings"
)

// settingsAdapter 把 internal/settings.Service 适配为 server.SettingsService，
// 在装配根（cmd/server）完成类型/错误转换，保持 server 与 settings 包解耦。
type settingsAdapter struct{ svc *settings.Service }

var _ server.SettingsService = settingsAdapter{}

func (a settingsAdapter) Get(ctx context.Context) (server.SettingsView, error) {
	r, err := a.svc.Get(ctx)
	if err != nil {
		return server.SettingsView{}, err
	}
	return toServerView(r), nil
}

func (a settingsAdapter) Update(ctx context.Context, in server.SettingsUpdate) (server.SettingsView, error) {
	keys := make([]domain.RuntimeSettingKey, 0, len(in.ResetToDefault))
	for _, k := range in.ResetToDefault {
		keys = append(keys, domain.RuntimeSettingKey(k))
	}
	r, err := a.svc.Update(ctx, settings.UpdateInput{
		DownloadMaxRetry: in.DownloadMaxRetry,
		ScrapeFullURL:    in.ScrapeFullURL,
		TownSkipRegex:    in.TownSkipRegex,
		ResetToDefault:   keys,
	})
	if err != nil {
		// 校验失败映射为客户端错误（400）并回显日语提示；其余为内部错误（500）。
		if settings.IsValidation(err) {
			return server.SettingsView{}, &server.SettingsError{Validation: true, Message: err.Error()}
		}
		return server.SettingsView{}, err
	}
	return toServerView(r), nil
}

func toServerView(r settings.Resolved) server.SettingsView {
	return server.SettingsView{
		DownloadMaxRetry:        r.DownloadMaxRetry,
		DownloadMaxRetryDefault: r.DownloadMaxRetryDefault,
		DownloadMaxRetryOver:    r.DownloadMaxRetryOver,
		ScrapeFullURL:           r.ScrapeFullURL,
		ScrapeFullURLDefault:    r.ScrapeFullURLDefault,
		ScrapeFullURLOver:       r.ScrapeFullURLOver,
		TownSkipRegex:           r.TownSkipRegex,
		TownSkipRegexDefault:    r.TownSkipRegexDefault,
		TownSkipRegexOver:       r.TownSkipRegexOver,
	}
}
