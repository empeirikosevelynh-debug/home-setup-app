package inspect

import (
	"golden-gate-setup/internal/domain"
	"reflect"
	"testing"
)

func TestChocoPackagesParsed(t *testing.T) {
	got, e := readChocoPackages("chocolatey|2.4.1\r\nGit|2.56.0\r\nstarship|1.26.0\r\n\r\n")
	want := map[string]domain.InstalledPackage{"choco:chocolatey": {Version: "2.4.1"}, "choco:git": {Version: "2.56.0"}, "choco:starship": {Version: "1.26.0"}}
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, e)
	}
	for _, bad := range []string{"Chocolatey v2.4.1", "git|", "|1.0", "a|b|c"} {
		if _, e := readChocoPackages(bad); e == nil {
			t.Fatalf("malformed line accepted: %q", bad)
		}
	}
}
func TestWindowsAppsMatchedByName(t *testing.T) {
	entries := []UninstallEntry{{Name: "Zed 0.233.10", Location: `C:\Program Files\Zed`}, {Name: "Warp", Location: `C:\Program Files\Warp`}, {Name: "Zedd tools"}, {Name: "Cloudflare WARP"}}
	apps := windowsApps(entries, map[string]domain.InstalledPackage{"choco:warp-terminal": {Version: "1"}})
	want := []domain.AppBundle{{Name: "Warp", Path: `C:\Program Files\Warp`, Registered: true}, {Name: "Zed", Path: `C:\Program Files\Zed`}}
	if !reflect.DeepEqual(apps, want) {
		t.Fatalf("%+v", apps)
	}
}
