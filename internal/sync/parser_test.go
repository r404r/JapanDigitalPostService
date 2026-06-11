package sync

import (
	"strings"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// 真实 utf_ken_all 采样行（含特例：以下に掲載がない場合 / 丁目 / 一町域多邮编 / 引号内全角括号）。
const sampleCSV = `01101,"060  ","0600000","ホッカイドウ","サッポロシチュウオウク","イカニケイサイガナイバアイ","北海道","札幌市中央区","以下に掲載がない場合",0,0,0,0,0,0
01101,"064  ","0640941","ホッカイドウ","サッポロシチュウオウク","アサヒガオカ","北海道","札幌市中央区","旭ケ丘",0,0,1,0,0,0
01101,"060  ","0600042","ホッカイドウ","サッポロシチュウオウク","オオドオリニシ（１−１９チョウメ）","北海道","札幌市中央区","大通西（１〜１９丁目）",1,0,1,0,0,0
`

func parseAll(t *testing.T, csv string) []domain.Address {
	t.Helper()
	var got []domain.Address
	n, err := ParseStream(strings.NewReader(csv), func(a *domain.Address) error {
		got = append(got, *a)
		return nil
	})
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	if n != len(got) {
		t.Fatalf("count %d != emitted %d", n, len(got))
	}
	return got
}

func TestParseFields(t *testing.T) {
	got := parseAll(t, sampleCSV)
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}

	r0 := got[0]
	if r0.Zipcode != "0600000" || r0.JISCode != "01101" {
		t.Errorf("row0 zip/jis = %q/%q", r0.Zipcode, r0.JISCode)
	}
	if r0.Prefecture != "北海道" || r0.City != "札幌市中央区" || r0.Town != "以下に掲載がない場合" {
		t.Errorf("row0 kanji = %q/%q/%q", r0.Prefecture, r0.City, r0.Town)
	}
	if r0.PrefectureKana != "ホッカイドウ" || r0.TownKana != "イカニケイサイガナイバアイ" {
		t.Errorf("row0 kana = %q/%q", r0.PrefectureKana, r0.TownKana)
	}

	// 丁目标志位。
	if got[1].FlagChome != 1 {
		t.Errorf("row1 FlagChome = %d, want 1", got[1].FlagChome)
	}
	// 一町域多邮编标志位 + 町域名含全角括号/丁目。
	if got[2].FlagMultiZip != 1 {
		t.Errorf("row2 FlagMultiZip = %d, want 1", got[2].FlagMultiZip)
	}
	if !strings.Contains(got[2].Town, "丁目") {
		t.Errorf("row2 town = %q, want contains 丁目", got[2].Town)
	}
	if r0.SourceHash == "" || len(r0.SourceHash) != 64 {
		t.Errorf("row0 source_hash invalid: %q", r0.SourceHash)
	}
}

// 更新区分/变更理由（第14/15列）属差分元数据，不应影响 source_hash，
// 否则同一地址经 full 与 diff add 导入会被误判为 updated，破坏跨路径幂等。
func TestHashIgnoresDiffMetadata(t *testing.T) {
	full := `15210,"948  ","9480013","ニイガタケン","トオカマチシ","カワラチョウ","新潟県","十日町市","川原町",0,0,0,0,0,0`
	diff := `15210,"948  ","9480013","ニイガタケン","トオカマチシ","カワラチョウ","新潟県","十日町市","川原町",0,0,0,0,1,5`
	a := parseAll(t, full)[0]
	b := parseAll(t, diff)[0]
	if a.SourceHash != b.SourceHash {
		t.Errorf("hash differs across update flags: %s vs %s", a.SourceHash, b.SourceHash)
	}
	if a.Key() != b.Key() {
		t.Errorf("key differs: %+v vs %+v", a.Key(), b.Key())
	}
}

func TestParseRejectsShortRecord(t *testing.T) {
	_, err := ParseStream(strings.NewReader("01101,foo,bar\n"), func(*domain.Address) error { return nil })
	if err == nil {
		t.Fatal("want error on short record, got nil")
	}
}
