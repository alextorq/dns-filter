package settings

import "testing"

func TestValidatePositiveInt(t *testing.T) {
	if err := ValidatePositiveInt("1"); err != nil {
		t.Errorf("1 should be valid: %v", err)
	}
	if err := ValidatePositiveInt("0"); err == nil {
		t.Error("0 must be rejected")
	}
	if err := ValidatePositiveInt("-3"); err == nil {
		t.Error("negative must be rejected")
	}
	if err := ValidatePositiveInt("nope"); err == nil {
		t.Error("non-integer must be rejected")
	}
}

func TestValidateIntRange(t *testing.T) {
	v := ValidateIntRange(1, 3650)

	// Happy path: in-range values (lower bound, a typical value, upper bound).
	for _, ok := range []string{"1", "30", "3650", " 30 "} {
		if err := v(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}

	// Negative: below the lower bound, the upper bound + 1, and a non-integer.
	for _, bad := range []string{"0", "-1", "3651", "abc", ""} {
		if err := v(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestValidateSecret(t *testing.T) {
	// Happy path: ≥8 символов, разные алфавиты и пробелы вокруг.
	for _, ok := range []string{
		"abcdefgh",
		"12345678",
		"  longerKeyWithSpaces  ",
		"AAAAAAAA-BBBBBBBB-CCCCCCCC", // типовой VT-формат
	} {
		if err := ValidateSecret(ok); err != nil {
			t.Errorf("%q должно проходить: %v", ok, err)
		}
	}

	// Негатив: пустые/слишком короткие — характерные ошибки копипасты.
	for _, bad := range []string{
		"",       // полностью пусто — это путь к Reset, не Set
		"   ",    // пробелы трактуются как пустота после Trim
		"short7", // ровно 7 < 8
		"a",      // мусор
	} {
		if err := ValidateSecret(bad); err == nil {
			t.Errorf("%q должно отклоняться", bad)
		}
	}
}

func TestParseSecret_Trims(t *testing.T) {
	if got := ParseSecret("  some-key-with-newline\n"); got != "some-key-with-newline" {
		t.Errorf("ParseSecret = %q, want trimmed value", got)
	}
}
