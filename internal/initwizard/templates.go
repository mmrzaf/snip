package initwizard

// BuildDefaultPlan returns a default build plan based on detected signals.
// This is a placeholder for future extension; currently not used.
func BuildDefaultPlan(signals ProjectSignals) BuildPlan {
	plan := BuildPlan{
		DefaultProfile: "full",
		Slices:         map[string][]string{},
		Profiles:       map[string][]string{},
	}

	plan.Slices["docs"] = []string{
		"README.md",
		"docs/**",
	}

	if signals.Go {
		plan.Slices["api"] = []string{
			"cmd/**/*.go",
			"internal/**/*.go",
		}

		plan.Slices["tests"] = []string{
			"**/*_test.go",
		}
	}

	if signals.Node {
		plan.Slices["frontend"] = []string{
			"src/**",
			"app/**",
		}
	}

	plan.Profiles["full"] = []string{
		"api",
		"docs",
	}

	plan.Profiles["review"] = []string{
		"api",
	}

	return plan
}
