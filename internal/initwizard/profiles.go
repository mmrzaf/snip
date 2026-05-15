package initwizard

import (
	"sort"

	"github.com/mmrzaf/snip/internal/config"
)

func buildProfiles(proj ProjectInfo, slices map[string]config.SliceConfig) map[string]config.Profile {
	defaultEnable := []string{}
	if _, ok := slices["code"]; ok {
		defaultEnable = append(defaultEnable, "code")
	}
	for _, s := range []string{"docs", "configs"} {
		if _, ok := slices[s]; ok {
			defaultEnable = append(defaultEnable, s)
		}
	}
	if proj.IsWebApp {
		for _, s := range []string{"components", "pages"} {
			if _, ok := slices[s]; ok && !contains(defaultEnable, s) {
				defaultEnable = append(defaultEnable, s)
			}
		}
	}
	if len(defaultEnable) == 0 {
		for n := range slices {
			defaultEnable = []string{n}
			break
		}
	}

	fullEnable := make([]string, 0, len(slices))
	for n := range slices {
		fullEnable = append(fullEnable, n)
	}
	sort.Strings(fullEnable)

	var minimalEnable []string
	if _, ok := slices["code"]; ok {
		minimalEnable = []string{"code"}
	} else if len(defaultEnable) > 0 {
		minimalEnable = defaultEnable[:1]
	} else {
		minimalEnable = []string{}
	}

	debugEnable := defaultEnable
	if _, ok := slices["tests"]; ok && !contains(debugEnable, "tests") {
		debugEnable = append(debugEnable, "tests")
	}

	return map[string]config.Profile{
		"default": {Enable: defaultEnable},
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
