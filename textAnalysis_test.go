package mastobots

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseJapaneseUsesSudachiAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/analyze" {
			t.Errorf("request = %s %s, want POST /v1/analyze", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var request sudachiAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Text != "東京都へ行く" || request.Mode != "B" {
			t.Errorf("request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tokens": [
				{"surface":"東京都","part_of_speech":["名詞","固有名詞","地名","一般","*","*"],"normalized_form":"東京都","dictionary_form":"東京都","reading_form":"トウキョウト","dictionary_id":0,"synonym_group_ids":[1,2],"oov":false},
				{"surface":"へ","part_of_speech":["助詞","格助詞","*","*","*","*"],"normalized_form":"へ","dictionary_form":"へ","reading_form":"ヘ","dictionary_id":0,"synonym_group_ids":[],"oov":false},
				{"surface":"行く","part_of_speech":["動詞","非自立可能","*","*","五段-カ行","終止形-一般"],"normalized_form":"行く","dictionary_form":"行く","reading_form":"イク","dictionary_id":0,"synonym_group_ids":[],"oov":false}
			],
			"count": 3,
			"mode": "B"
		}`))
	}))
	defer server.Close()

	client, err := newSudachiClient(server.URL+"/v1/analyze", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseJapanese(client, "東京都へ行く")
	if err != nil {
		t.Fatalf("parseJapanese() error = %v", err)
	}
	if result.length() != 3 {
		t.Fatalf("result.length() = %d, want 3", result.length())
	}
	if !result.Nodes[0].isPlaceName() {
		t.Error("東京都 should be a place name")
	}
	if result.Nodes[0].reading != "とうきょうと" || result.Nodes[0].dictionaryForm != "東京都" {
		t.Errorf("full token forms = reading:%q dictionary:%q", result.Nodes[0].reading, result.Nodes[0].dictionaryForm)
	}
	if result.Nodes[0].dictionaryID != 0 || result.Nodes[0].isOOV {
		t.Errorf("dictionary fields = id:%d oov:%t", result.Nodes[0].dictionaryID, result.Nodes[0].isOOV)
	}
	if got := result.Nodes[0].synonymGroupIDs; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("synonymGroupIDs = %#v, want [1 2]", got)
	}
	if !result.contain("行く") {
		t.Error("result should contain normalized form 行く")
	}

	candidates := result.candidates()
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].surface != "東京都" || candidates[0].firstKana != "と" {
		t.Errorf("candidate = %#v, want surface 東京都 and first kana と", candidates[0])
	}
	if locations := result.getWeatherQueryLocation(); len(locations) != 1 || locations[0] != "東京都" {
		t.Errorf("getWeatherQueryLocation() = %#v, want [東京都]", locations)
	}
}

func TestParseJapaneseReportsSudachiAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"error":{"code":"analysis_timeout","message":"Sudachi analysis did not finish before the timeout"}}`))
	}))
	defer server.Close()

	client, err := newSudachiClient(server.URL+"/v1/analyze", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseJapanese(client, "待つ")
	if err == nil || !strings.Contains(err.Error(), "analysis_timeout") || !strings.Contains(err.Error(), "HTTP 504") {
		t.Fatalf("parseJapanese() error = %v", err)
	}
}

func TestParseJapanesePreservesOOVFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"tokens":[{
				"surface":"kami",
				"part_of_speech":["名詞","普通名詞","一般","*","*","*"],
				"normalized_form":"kami",
				"dictionary_form":"kami",
				"reading_form":"",
				"dictionary_id":-1,
				"synonym_group_ids":[12,34],
				"oov":true
			}],
			"count":1,
			"mode":"B"
		}`))
	}))
	defer server.Close()

	client, err := newSudachiClient(server.URL+"/v1/analyze", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseJapanese(client, "kami")
	if err != nil {
		t.Fatal(err)
	}
	node := result.Nodes[0]
	if node.dictionaryID != -1 || !node.isOOV || node.reading != "kami" {
		t.Errorf("OOV token = %#v", node)
	}
	if got := node.synonymGroupIDs; len(got) != 2 || got[0] != 12 || got[1] != 34 {
		t.Errorf("synonymGroupIDs = %#v, want [12 34]", got)
	}
}

func TestParseJapaneseRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{`},
		{name: "wrong mode", body: `{"tokens":[],"count":0,"mode":"A"}`},
		{name: "count mismatch", body: `{"tokens":[],"count":1,"mode":"B"}`},
		{name: "empty tokens", body: `{"tokens":[],"count":0,"mode":"B"}`},
		{name: "old short token", body: `{"tokens":[{"surface":"語","part_of_speech":["名詞","普通名詞","一般","*","*","*"],"normalized_form":"語"}],"count":1,"mode":"B"}`},
		{name: "invalid POS", body: `{"tokens":[{"surface":"語","part_of_speech":["名詞"],"normalized_form":"語"}],"count":1,"mode":"B"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client, err := newSudachiClient(server.URL+"/v1/analyze", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseJapanese(client, "語"); err == nil {
				t.Error("parseJapanese() error = nil, want invalid response error")
			}
		})
	}
}

func TestParseJapaneseHonorsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tokens":[],"count":0,"mode":"B"}`))
	}))
	defer server.Close()

	client, err := newSudachiClient(server.URL+"/v1/analyze", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseJapanese(client, "待つ"); err == nil {
		t.Error("parseJapanese() error = nil, want timeout error")
	}
}

func TestSudachiArticleTitleCandidates(t *testing.T) {
	result := sudachiResult{Nodes: []sudachiMorpheme{
		newSudachiMorpheme("正義", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("戦争", "名詞", "普通名詞", "サ変可能"),
		newSudachiMorpheme("絶対", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("101", "名詞", "数詞", "*"),
		newSudachiMorpheme("地獄", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("発掘", "名詞", "普通名詞", "サ変可能"),
		newSudachiMorpheme("証言", "名詞", "普通名詞", "サ変可能"),
	}}
	result.Nodes[0].reading = "せいぎ"

	candidates := result.candidates()
	if len(candidates) != 6 {
		t.Fatalf("len(candidates) = %d, want 6", len(candidates))
	}
	if candidates[0].surface != "正義" || candidates[0].firstKana != "せ" {
		t.Errorf("first candidate = %#v, want 正義/せ", candidates[0])
	}
}

func TestJumanCompatibleFormalNounsAreNotCandidates(t *testing.T) {
	result := sudachiResult{Nodes: []sudachiMorpheme{
		newSudachiMorpheme("問題", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("こと", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("もの", "名詞", "普通名詞", "サ変可能"),
		newSudachiMorpheme("もん", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("わけ", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("はず", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("つもり", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("の", "助詞", "準体助詞", "*"),
		newSudachiMorpheme("ん", "助詞", "準体助詞", "*"),
	}}

	candidates := result.candidates()
	if len(candidates) != 1 || candidates[0].surface != "問題" {
		t.Errorf("candidates = %#v, want only 問題", candidates)
	}
}

func TestNounsOutsideJumanFormalNounDefinitionRemainCandidates(t *testing.T) {
	result := sudachiResult{Nodes: []sudachiMorpheme{
		newSudachiMorpheme("ところ", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("ため", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("とき", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("うち", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("ほう", "名詞", "普通名詞", "一般"),
		newSudachiMorpheme("まま", "名詞", "普通名詞", "副詞可能"),
		newSudachiMorpheme("そのまま", "名詞", "普通名詞", "副詞可能"),
	}}

	if candidates := result.candidates(); len(candidates) != 7 {
		t.Errorf("len(candidates) = %d, want 7: %#v", len(candidates), candidates)
	}
}

func TestSudachiNormalizedFormDeterminesWeatherDate(t *testing.T) {
	tests := []struct {
		normalized string
		want       int
	}{
		{normalized: "明日", want: 1},
		{normalized: "明後日", want: 2},
		{normalized: "今", want: -1},
		{normalized: "現在", want: -1},
	}
	for _, test := range tests {
		result := sudachiResult{Nodes: []sudachiMorpheme{{surface: test.normalized, normalizedForm: test.normalized}}}
		if got := result.getWeatherQueryDate(); got != test.want {
			t.Errorf("getWeatherQueryDate(%q) = %d, want %d", test.normalized, got, test.want)
		}
	}
}

func TestNewSudachiClient(t *testing.T) {
	client, err := newSudachiClient("http://192.0.2.1:8080/v1/analyze", 0)
	if err != nil {
		t.Fatalf("newSudachiClient() error = %v", err)
	}
	if client.endpoint != "http://192.0.2.1:8080/v1/analyze" {
		t.Errorf("endpoint = %q", client.endpoint)
	}
	if client.httpClient.Timeout != defaultSudachiAPITimeout {
		t.Errorf("timeout = %s, want %s", client.httpClient.Timeout, defaultSudachiAPITimeout)
	}
}

func TestNewSudachiClientRejectsInvalidURL(t *testing.T) {
	for _, endpoint := range []string{"", "192.0.2.1:8080/v1/analyze", "ftp://example.com/v1/analyze", "http://example.com/healthz", "http://user@example.com/v1/analyze", "http://example.com/v1/analyze?q=1"} {
		if _, err := newSudachiClient(endpoint, time.Second); err == nil {
			t.Errorf("newSudachiClient(%q) error = nil", endpoint)
		}
	}
}

func newSudachiMorpheme(surface, pos0, pos1, pos2 string) sudachiMorpheme {
	return sudachiMorpheme{
		surface:        surface,
		partOfSpeech:   [6]string{pos0, pos1, pos2, "*", "*", "*"},
		normalizedForm: surface,
		dictionaryForm: surface,
		reading:        surface,
	}
}
