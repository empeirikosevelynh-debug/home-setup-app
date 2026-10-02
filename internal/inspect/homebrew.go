package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/domain"
	"strings"
)

func readPackages(data []byte) (map[string]domain.InstalledPackage, error) {
	var info struct {
		Formulae *[]struct {
			Name      string
			FullName  string `json:"full_name"`
			LinkedKeg string `json:"linked_keg"`
			Installed []struct {
				Version   string
				OnRequest *bool `json:"installed_on_request"`
			}
		} `json:"formulae"`
		Casks *[]struct {
			Token, Tap string
			Installed  json.RawMessage
		} `json:"casks"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("cannot read installed Homebrew metadata: %w", err)
	}
	if info.Formulae == nil || info.Casks == nil {
		return nil, fmt.Errorf("incomplete installed Homebrew metadata")
	}
	packages := map[string]domain.InstalledPackage{}
	for _, f := range *info.Formulae {
		if f.Name == "" || len(f.Installed) == 0 || f.Installed[0].Version == "" {
			return nil, fmt.Errorf("invalid installed formula metadata")
		}
		key := f.FullName
		if key == "" {
			key = f.Name
		}
		key = strings.TrimPrefix(key, "homebrew/core/")
		installed := f.Installed[0]
		for _, v := range f.Installed {
			if v.Version == f.LinkedKeg {
				installed = v
			}
		}
		packages["formula:"+key] = domain.InstalledPackage{Version: installed.Version, OnRequest: installed.OnRequest}
	}
	for _, c := range *info.Casks {
		var v string
		if c.Token == "" || bytes.Equal(c.Installed, []byte("null")) || json.Unmarshal(c.Installed, &v) != nil || v == "" {
			return nil, fmt.Errorf("invalid installed cask metadata")
		}
		key := c.Token
		if c.Tap != "" && c.Tap != "homebrew/cask" {
			key = c.Tap + "/" + c.Token
		}
		packages["cask:"+key] = domain.InstalledPackage{Version: v}
	}
	return packages, nil
}
