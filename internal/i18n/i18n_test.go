package i18n

import "testing"

func TestCatalogParity(t *testing.T) {
	if err := Parity(); err != nil {
		t.Fatal(err)
	}
}

func TestChineseLocalesUseTraditional(t *testing.T) {
	if !isChinese("zh-TW") || !isChinese("zh_CN.UTF-8") || !isChinese("Chinese (Traditional)") {
		t.Fatal("chinese locales")
	}
	if isChinese("en-US") || isChinese("C") || isChinese("") {
		t.Fatal("non-chinese locales")
	}
}

func TestTFallsBack(t *testing.T) {
	if T(En, "welcome.choose") != "Choose a folder" {
		t.Fatal(T(En, "welcome.choose"))
	}
	if T(ZhHant, "welcome.choose") != "選擇資料夾" {
		t.Fatal(T(ZhHant, "welcome.choose"))
	}
	if T("fr", "welcome.choose") != "Choose a folder" {
		t.Fatal("fallback")
	}
}
