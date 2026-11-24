package i18n

import (
	"regexp"
	"sort"
	"testing"
)

func TestCatalogParity(t *testing.T) {
	if err := Parity(); err != nil {
		t.Fatal(err)
	}
}

func TestFormatVerbParity(t *testing.T) {
	re := regexp.MustCompile(`%(?:\.\d+)?[dfs]`)
	verbs := func(s string) string {
		m := re.FindAllString(s, -1)
		sort.Strings(m)
		out := ""
		for _, v := range m {
			out += v + "|"
		}
		return out
	}
	for _, p := range pairs {
		vZh := verbs(p.zh)
		vEn := verbs(p.en)
		vJa := verbs(p.ja)
		if vZh != vEn || vZh != vJa {
			t.Errorf("format verbs mismatch for key %q: zh=%q en=%q ja=%q", p.key, vZh, vEn, vJa)
		}
	}
}

func TestChineseLocalesUseTraditional(t *testing.T) {
	if !isChinese("zh-TW") || !isChinese("zh_CN.UTF-8") || !isChinese("Chinese (Traditional)") {
		t.Fatal("chinese locales")
	}
	if isChinese("en-US") || isChinese("ja-JP") || isChinese("C") || isChinese("") {
		t.Fatal("non-chinese locales")
	}
}

func TestJapaneseLocales(t *testing.T) {
	if !isJapanese("ja-JP") || !isJapanese("ja_JP.UTF-8") || !isJapanese("Japanese") {
		t.Fatal("japanese locales")
	}
	if isJapanese("en-US") || isJapanese("zh-TW") || isJapanese("C") || isJapanese("") {
		t.Fatal("non-japanese locales")
	}
}

func TestTFallsBack(t *testing.T) {
	if T(En, "welcome.choose") != "Choose a folder" {
		t.Fatal(T(En, "welcome.choose"))
	}
	if T(ZhHant, "welcome.choose") != "選擇資料夾" {
		t.Fatal(T(ZhHant, "welcome.choose"))
	}
	if T(Ja, "welcome.choose") != "フォルダを選択" {
		t.Fatal(T(Ja, "welcome.choose"))
	}
	if T("fr", "welcome.choose") != "Choose a folder" {
		t.Fatal("fallback")
	}
}
