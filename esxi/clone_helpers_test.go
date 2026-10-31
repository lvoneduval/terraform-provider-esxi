package esxi

import "testing"

func TestValidateName(t *testing.T) {
	ok := []string{"vm1", "web-01", "Debian 12", "template_2026.10"}
	bad := []string{"", "..", "../x", "a/b", "a\"b", "a$(id)", "a`id`", "a;b", "a'b", ".hidden", "-x", "a..b", "a\\b", "a&b"}
	for _, v := range ok {
		if err := validateName("n", v); err != nil {
			t.Errorf("%q should be accepted: %s", v, err)
		}
	}
	for _, v := range bad {
		if err := validateName("n", v); err == nil {
			t.Errorf("%q should be refused", v)
		}
	}
}

func TestValidateRemotePath(t *testing.T) {
	ok := []string{"/vmfs/volumes/ds1/templates/debian/debian.vmdk"}
	bad := []string{"", "relative/x", "/vmfs/../etc", "/a\"b", "/a$(id)", "/a;b", "/a'b"}
	for _, v := range ok {
		if err := validateRemotePath("p", v); err != nil {
			t.Errorf("%q should be accepted: %s", v, err)
		}
	}
	for _, v := range bad {
		if err := validateRemotePath("p", v); err == nil {
			t.Errorf("%q should be refused", v)
		}
	}
}

func TestValidateTemplatePath(t *testing.T) {
	if err := validateTemplatePath("/vmfs/volumes/ds1/templates/debian"); err != nil {
		t.Errorf("valid template path refused: %s", err)
	}
	bad := []string{
		"", "/", "/vmfs/volumes", "/vmfs/volumes/ds1", "/vmfs/volumes/ds1/templates", "/vmfs/volumes/ds1/templates/",
		"/vmfs/volumes/ds1/templates/..", "/etc/templates/x", "/vmfs/volumes/ds1/other/x",
	}
	for _, v := range bad {
		if err := validateTemplatePath(v); err == nil {
			t.Errorf("%q should be refused before rm -rf", v)
		}
	}
}

func TestSedEscape(t *testing.T) {
	if got := sedEscape("a.b"); got != "a\\.b" {
		t.Errorf("got %q", got)
	}
}
