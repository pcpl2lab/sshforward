package configedit

import "testing"

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"gitea", "db-1.prod_x"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "my svc", "a/b", "ż", "null", "Null", "NULL", "~"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
}

func TestValidateHost(t *testing.T) {
	for _, ok := range []string{"", "10.0.0.5", "db.internal"} {
		if err := ValidateHost(ok); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"a b", "host;rm"} {
		if err := ValidateHost(bad); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want error", bad)
		}
	}
}

func TestParsePort(t *testing.T) {
	for in, want := range map[string]int{"3306": 3306, " 22 ": 22} {
		got, err := ParsePort(in)
		if err != nil || got != want {
			t.Errorf("ParsePort(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "65536", "abc", ""} {
		if _, err := ParsePort(bad); err == nil {
			t.Errorf("ParsePort(%q) = nil error, want error", bad)
		}
	}
}

func TestValidate_LeadingDash(t *testing.T) {
	// A plain "-" starts a block sequence entry: the result would not load.
	for _, bad := range []string{"-", "-x", "--"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
		if err := ValidateHost(bad); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want error", bad)
		}
	}
	for _, ok := range []string{"a-", "db-1"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
		if err := ValidateHost(ok); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", ok, err)
		}
	}
}
