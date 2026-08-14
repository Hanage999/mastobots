package mastobots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSudachiOutput(t *testing.T) {
	output := []byte("東京都\t名詞,固有名詞,地名,一般,*,*\t東京都\t東京都\tトウキョウト\t0\t[1, 2]\t\n" +
		"へ\t助詞,格助詞,*,*,*,*\tへ\tへ\tヘ\t0\t[]\t\n" +
		"行く\t動詞,非自立可能,*,*,五段-カ行,終止形-一般\t行く\t行く\tイク\t0\t[]\t\n" +
		"EOS\n")

	result, err := parseSudachiOutput(output)
	if err != nil {
		t.Fatalf("parseSudachiOutput() error = %v", err)
	}
	if result.length() != 3 {
		t.Fatalf("result.length() = %d, want 3", result.length())
	}
	if got := result.Nodes[0].reading; got != "とうきょうと" {
		t.Errorf("reading = %q, want %q", got, "とうきょうと")
	}
	if !result.Nodes[0].isPlaceName() {
		t.Error("東京都 should be a place name")
	}
	if result.Nodes[0].dictionaryID != 0 {
		t.Errorf("dictionaryID = %d, want 0", result.Nodes[0].dictionaryID)
	}
	if got := result.Nodes[0].synonymGroupIDs; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("synonymGroupIDs = %#v, want [1 2]", got)
	}
	if result.Nodes[0].isOOV {
		t.Error("東京都 should not be OOV")
	}
	if !result.contain("行く") {
		t.Error("result should contain dictionary form 行く")
	}

	candidates := result.candidates()
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].surface != "東京都" || candidates[0].firstKana != "と" {
		t.Errorf("candidate = %#v, want surface 東京都 and firstKana と", candidates[0])
	}
	if locations := result.getWeatherQueryLocation(); len(locations) != 1 || locations[0] != "東京都" {
		t.Errorf("getWeatherQueryLocation() = %#v, want [東京都]", locations)
	}
}

func TestParseSudachiOOV(t *testing.T) {
	output := []byte("阿quei\t名詞,普通名詞,一般,*,*,*\t阿quei\t阿quei\tアクエイ\t-1\t[]\t(OOV)\nEOS\n")
	result, err := parseSudachiOutput(output)
	if err != nil {
		t.Fatalf("parseSudachiOutput() error = %v", err)
	}
	if !result.Nodes[0].isOOV || result.Nodes[0].dictionaryID != -1 {
		t.Errorf("OOV fields = isOOV:%t dictionaryID:%d", result.Nodes[0].isOOV, result.Nodes[0].dictionaryID)
	}
}

func TestSudachiArticleTitleCandidates(t *testing.T) {
	output := []byte("「\t補助記号,括弧開,*,*,*,*\t「\t「\t「\t0\t[]\t\n" +
		"正義\t名詞,普通名詞,一般,*,*,*\t正義\t正義\tセイギ\t0\t[]\t\n" +
		"戦争\t名詞,普通名詞,サ変可能,*,*,*\t戦争\t戦争\tセンソウ\t0\t[14268]\t\n" +
		"絶対\t名詞,普通名詞,副詞可能,*,*,*\t絶対\t絶対\tゼッタイ\t0\t[22498]\t\n" +
		"101\t名詞,数詞,*,*,*,*\t101\t101\tイチレイイチ\t-1\t[]\t\n" +
		"地獄\t名詞,普通名詞,一般,*,*,*\t地獄\t地獄\tジゴク\t0\t[18776]\t\n" +
		"発掘\t名詞,普通名詞,サ変可能,*,*,*\t発掘\t発掘\tハックツ\t0\t[]\t\n" +
		"証言\t名詞,普通名詞,サ変可能,*,*,*\t証言\t証言\tショウゲン\t0\t[]\t\n" +
		"EOS\n")

	result, err := parseSudachiOutput(output)
	if err != nil {
		t.Fatalf("parseSudachiOutput() error = %v", err)
	}
	candidates := result.candidates()
	if len(candidates) != 6 {
		t.Fatalf("len(candidates) = %d, want 6", len(candidates))
	}
	if candidates[0].surface != "正義" || candidates[0].firstKana != "せ" {
		t.Errorf("first candidate = %#v, want 正義/せ", candidates[0])
	}
}

func TestJumanCompatibleFormalNounsAreNotCandidates(t *testing.T) {
	output := []byte("問題\t名詞,普通名詞,一般,*,*,*\t問題\t問題\tモンダイ\t0\t[]\t\n" +
		"こと\t名詞,普通名詞,一般,*,*,*\tこと\tこと\tコト\t0\t[]\t\n" +
		"もの\t名詞,普通名詞,サ変可能,*,*,*\t物\tもの\tモノ\t0\t[]\t\n" +
		"もん\t名詞,普通名詞,一般,*,*,*\tもん\tもん\tモン\t0\t[]\t\n" +
		"わけ\t名詞,普通名詞,一般,*,*,*\t訳\tわけ\tワケ\t0\t[4]\t\n" +
		"はず\t名詞,普通名詞,一般,*,*,*\t筈\tはず\tハズ\t0\t[]\t\n" +
		"つもり\t名詞,普通名詞,一般,*,*,*\t積もり\tつもり\tツモリ\t0\t[]\t\n" +
		"の\t助詞,準体助詞,*,*,*,*\tの\tの\tノ\t0\t[]\t\n" +
		"ん\t助詞,準体助詞,*,*,*,*\tの\tん\tン\t0\t[]\t\n" +
		"EOS\n")

	result, err := parseSudachiOutput(output)
	if err != nil {
		t.Fatalf("parseSudachiOutput() error = %v", err)
	}
	candidates := result.candidates()
	if len(candidates) != 1 || candidates[0].surface != "問題" {
		t.Errorf("candidates = %#v, want only 問題", candidates)
	}
}

func TestNounsOutsideJumanFormalNounDefinitionRemainCandidates(t *testing.T) {
	output := []byte("ところ\t名詞,普通名詞,副詞可能,*,*,*\t所\tところ\tトコロ\t0\t[]\t\n" +
		"ため\t名詞,普通名詞,副詞可能,*,*,*\t為\tため\tタメ\t0\t[]\t\n" +
		"とき\t名詞,普通名詞,副詞可能,*,*,*\t時\tとき\tトキ\t0\t[]\t\n" +
		"うち\t名詞,普通名詞,副詞可能,*,*,*\tうち\tうち\tウチ\t0\t[]\t\n" +
		"ほう\t名詞,普通名詞,一般,*,*,*\t方\tほう\tホウ\t0\t[]\t\n" +
		"まま\t名詞,普通名詞,副詞可能,*,*,*\t侭\tまま\tママ\t0\t[]\t\n" +
		"そのまま\t名詞,普通名詞,副詞可能,*,*,*\tそのまま\tそのまま\tソノママ\t0\t[]\t\n" +
		"EOS\n")

	result, err := parseSudachiOutput(output)
	if err != nil {
		t.Fatalf("parseSudachiOutput() error = %v", err)
	}
	candidates := result.candidates()
	if len(candidates) != 7 {
		t.Errorf("len(candidates) = %d, want 7: %#v", len(candidates), candidates)
	}
}

func TestParseSudachiOutputRejectsMalformedLine(t *testing.T) {
	if _, err := parseSudachiOutput([]byte("invalid output\n")); err == nil {
		t.Error("parseSudachiOutput() error = nil, want malformed output error")
	}
}

func TestParseSudachiOutputRejectsEmptyResult(t *testing.T) {
	if _, err := parseSudachiOutput([]byte("EOS\n")); err == nil {
		t.Error("parseSudachiOutput() error = nil, want empty result error")
	}
}

func TestLoadSudachiSettings(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"sudachi-0.8.0.jar", "sudachi.json"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	settings, err := loadSudachiSettings(home)
	if err != nil {
		t.Fatalf("loadSudachiSettings() error = %v", err)
	}
	if settings.jarPath != filepath.Join(home, "sudachi-0.8.0.jar") {
		t.Errorf("jarPath = %q", settings.jarPath)
	}
	if settings.configPath != filepath.Join(home, "sudachi.json") {
		t.Errorf("configPath = %q", settings.configPath)
	}
}

func TestLoadSudachiSettingsRequiresHome(t *testing.T) {
	if _, err := loadSudachiSettings(""); err == nil {
		t.Error("loadSudachiSettings() error = nil, want missing SudachiHome error")
	}
}
