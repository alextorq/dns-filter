package easy_list

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// parseSorted runs ParseEasyList and returns the result sorted, so tests can
// compare against an order-independent expectation (map iteration order is
// not stable in Go).
func parseSorted(input string) []string {
	got := ParseEasyList(strings.NewReader(input))
	sort.Strings(got)
	return got
}

func TestParseEasyList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty input",
			input:    "",
			expected: []string{}, // Ожидаем пустой слайс (или nil)
		},
		{
			name: "Basic blocking",
			input: `
||example.com^
||test.org^
`,
			expected: []string{"example.com.", "test.org."},
		},
		{
			name: "Whitelist logic (Merge check)",
			input: `
||bad.com^
||good.com^
@@||good.com^
`,
			expected: []string{"bad.com."}, // good.com должен исчезнуть
		},
		{
			name: "Ignore paths and files",
			input: `
||site.com/ads/
||domain.com/banner.jpg
||clean-domain.com^
`,
			expected: []string{"clean-domain.com."},
		},
		{
			name: "Drop block rules with contextual options, keep ports",
			input: `
||options.com^$third-party,popup
||port-test.com:8080^
`,
			// options.com отбрасывается: $third-party,popup делает правило
			// контекстным, а не безусловной блокировкой всего домена.
			// Правило без опций выживает (порт по-прежнему срезается).
			expected: []string{"port-test.com."},
		},
		{
			name: "Ignore CSS rules and wildcards",
			input: `
##div.ad
example.com##.banner
||wildcard.*^
||valid.com^
`,
			expected: []string{"valid.com."},
		},
		{
			name: "Mixed complex case",
			input: `
! Comments
[Adblock Plus 2.0]
||blocked.com^
@@||friend.com^
||friend.com^$third-party
||trash.com/file
||weird-port.com:999^
`,
			expected: []string{"blocked.com.", "weird-port.com."},
		},
		{
			// Regression: ruadlist+easylist.txt contains rules like "||ru^$third-party"
			// that previously left a bare "ru." in block_lists, which then matched
			// every *.ru domain via subdomainAncestors and triggered mass auto-block.
			name: "Drop bare public suffixes (ICANN TLD / eTLD)",
			input: `
||ru^$third-party
||xyz^
||co.uk^
||ozone.ru^
||example.co.uk^
`,
			expected: []string{"ozone.ru.", "example.co.uk."},
		},
		{
			name: "Drop unknown single-label tokens that yield no eTLD+1",
			input: `
||localhost^
||internal^
||valid.tld.com^
`,
			expected: []string{"valid.tld.com."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем Reader из строки
			reader := strings.NewReader(tt.input)

			got := ParseEasyList(reader)

			// ВАЖНО: Сортируем полученный и ожидаемый списки,
			// так как порядок итерации по map в Go не гарантирован.
			sort.Strings(got)
			sort.Strings(tt.expected)

			// Сравниваем длины
			if len(got) != len(tt.expected) {
				t.Errorf("ParseEasyList() length = %v, want %v. \nGot: %v\nWant: %v", len(got), len(tt.expected), got, tt.expected)
				return
			}

			// Сравниваем элементы
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("ParseEasyList() mismatch at index %d \nGot: %s\nWant: %s", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// ---- isFlattenableModifierSet ----

// Blocking-правило ||domain^$opts можно уплощать в голую блокировку домена
// только если КАЖДЫЙ модификатор оставляет его безусловной блокировкой всего
// домена. Контекстные (domain=, third-party, popup), частичные (типы ресурсов)
// и меняющие действие (badfilter, dnsrewrite, csp, removeparam) модификаторы
// должны делать правило неуплощаемым.
func TestIsFlattenableModifierSet(t *testing.T) {
	cases := []struct {
		name    string
		options string
		want    bool
	}{
		{"no options", "", true},
		{"whitespace only", "   ", true},
		{"important alone", "important", true},
		{"all alone", "all", true},
		{"important and all", "important,all", true},
		{"case-insensitive important", "IMPORTANT", true},
		{"trailing comma tolerated", "important,", true},
		{"leading comma tolerated", ",important", true},

		{"domain= page restriction", "domain=dzen.ru", false},
		{"domain= multi value", "domain=a.ru|b.ru", false},
		{"third-party context", "third-party", false},
		{"3p alias", "3p", false},
		{"negated third-party", "~third-party", false},
		{"popup only", "popup", false},
		{"csp is not a block", "csp=script-src 'none'", false},
		{"resource type script", "script", false},
		{"resource type image", "image", false},
		{"document type", "document", false},
		{"badfilter disables a rule", "badfilter", false},
		{"dnsrewrite changes the answer", "dnsrewrite=0.0.0.0", false},
		{"removeparam is non-blocking", "removeparam=utm", false},
		{"unknown future modifier", "somethingnew", false},
		{"safe mixed with contextual", "important,third-party", false},
		{"contextual mixed with safe", "domain=x.ru,important", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFlattenableModifierSet(tc.options); got != tc.want {
				t.Fatalf("isFlattenableModifierSet(%q)=%v, want %v", tc.options, got, tc.want)
			}
		})
	}
}

// ---- block rules with $-options ----

// Дополняет TestIsFlattenableModifierSet на уровне всего парсера: правило
// с контекстными опциями должно исчезать из выдачи целиком, а не оседать
// голым доменом (root-cause инцидента с mail.ru).
func TestParseEasyList_BlockRuleModifiers(t *testing.T) {
	cases := []struct {
		name string
		rule string
		kept bool
	}{
		{"no options is kept", "||plain-block.com^", true},
		{"important is kept", "||tracker.com^$important", true},
		{"all is kept", "||tracker.com^$all", true},
		{"important+all kept", "||tracker.com^$important,all", true},
		{"empty options kept", "||tracker.com^$", true},

		{"domain= dropped", "||legit.com^$domain=othersite.com", false},
		{"third-party dropped", "||legit.com^$third-party", false},
		{"3p dropped", "||legit.com^$3p", false},
		{"negated third-party dropped", "||legit.com^$~third-party", false},
		{"popup dropped", "||legit.com^$popup", false},
		{"csp dropped", "||legit.com^$csp=frame-src 'none'", false},
		{"resource type dropped", "||legit.com^$script", false},
		{"badfilter dropped (must not block)", "||legit.com^$badfilter", false},
		{"dnsrewrite dropped", "||legit.com^$dnsrewrite=1.2.3.4", false},
		{"removeparam dropped", "||legit.com^$removeparam=utm", false},
		{"important+third-party dropped", "||legit.com^$important,third-party", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSorted(tc.rule)
			if tc.kept {
				if len(got) != 1 {
					t.Fatalf("rule %q: expected it kept, got %v", tc.rule, got)
				}
			} else if len(got) != 0 {
				t.Fatalf("rule %q: expected it dropped, got %v", tc.rule, got)
			}
		})
	}
}

// Exception (@@) правила снимают блокировку. Их опции игнорируются: более
// широкий whitelist может лишь НЕ заблокировать домен — ложно заблокировать
// легитимный он не способен, поэтому модификаторы exception-правил не делают
// их «неуплощаемыми» (в отличие от blocking-правил).
func TestParseEasyList_ExceptionsIgnoreModifiers(t *testing.T) {
	input := `
||blocked-a.com^
||blocked-b.com^
||blocked-c.com^
@@||blocked-a.com^
@@||blocked-b.com^$third-party
@@||blocked-c.com^$domain=somesite.com
`
	got := parseSorted(input)
	if len(got) != 0 {
		t.Fatalf("все три домена должны быть сняты whitelist'ом, got %v", got)
	}
}

// Нормализация домена: нижний регистр, срез anchor-символа '|', отсев
// IP-литералов. Модификаторы детектятся регистронезависимо.
func TestParseEasyList_Normalization(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{"uppercase domain is lowercased", "||EXAMPLE.COM^", []string{"example.com."}},
		{"mixed case domain", "||CDN.Example.COM^", []string{"cdn.example.com."}},
		{"trailing pipe anchor stripped", "||example.com^|", []string{"example.com."}},
		{"pipe before separator stripped", "||example.com|^", []string{"example.com."}},
		{"uppercase modifier still detected", "||legit.com^$THIRD-PARTY", []string{}},
		{"ipv4 literal dropped", "||8.8.8.8^", []string{}},
		{"ipv4 literal dropped even with safe modifier", "||8.8.8.8^$important", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSorted(tc.input)
			exp := append([]string(nil), tc.expected...)
			sort.Strings(exp)
			if !slices.Equal(got, exp) {
				t.Fatalf("got %v, want %v", got, exp)
			}
		})
	}
}

// Регрессия на инцидент: ruadlist содержит правила вида
// ||mail.ru^$domain=dzen.ru — браузерное «блокировать mail.ru, когда страница
// открыта на dzen.ru». Старый парсер срезал $... и оставлял голый mail.ru,
// который уезжал в block_lists, и DNS отдавал по mail.ru NXDOMAIN глобально.
func TestParseEasyList_ContextualRuleDoesNotPoisonBlocklist(t *testing.T) {
	input := `
||mail.ru^$domain=dzen.ru
||market.yandex.ru^$popup,domain=cq.ru
||jivosite.com^$domain=bagnet.org|medkrug.ru
||realtracker.com^
`
	got := parseSorted(input)
	want := []string{"realtracker.com."}
	if !slices.Equal(got, want) {
		t.Fatalf("контекстные правила отравили блок-лист: got %v, want %v", got, want)
	}
}
