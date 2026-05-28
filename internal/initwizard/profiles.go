package initwizard

import (
	"sort"

	"github.com/mmrzaf/snip/internal/config"
)

func buildProfiles(proj ProjectInfo, slices map[string]config.SliceConfig) map[string]config.Profile {
	apiEnable := []string{}
	if _, ok := slices["code"]; ok {
		apiEnable = append(apiEnable, "code")
	}
	for _, s := range []string{"docs", "configs"} {
		if _, ok := slices[s]; ok {
			apiEnable = append(apiEnable, s)
		}
	}
	if proj.IsWebApp {
		for _, s := range []string{"components", "pages"} {
			if _, ok := slices[s]; ok && !contains(apiEnable, s) {
				apiEnable = append(apiEnable, s)
			}
		}
	}
	if len(apiEnable) == 0 {
		for n := range slices {
			apiEnable = []string{n}
			break
		}
	}
	sort.Strings(apiEnable)

	fullEnable := make([]string, 0, len(slices))
	for n := range slices {
		fullEnable = append(fullEnable, n)
	}
	sort.Strings(fullEnable)

	minimalEnable := []string{}
	if _, ok := slices["code"]; ok {
		minimalEnable = []string{"code"}
	} else if len(apiEnable) > 0 {
		minimalEnable = []string{apiEnable[0]}
	}

	debugEnable := append([]string(nil), apiEnable...)
	if _, ok := slices["tests"]; ok && !contains(debugEnable, "tests") {
		debugEnable = append(debugEnable, "tests")
	}
	sort.Strings(debugEnable)

	return map[string]config.Profile{
		"api":     {Enable: apiEnable},
		"full":    {Enable: fullEnable},
		"minimal": {Enable: minimalEnable},
		"debug":   {Enable: debugEnable},
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
