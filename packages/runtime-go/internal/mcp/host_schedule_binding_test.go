package mcp

import "testing"

func TestHostScheduleMCPBindingOwnsExactLoopbackAndReadOnlyPolicy(t *testing.T) {
	configured := hostScheduleMCPBindingFixtureV1()
	bound := BindHostScheduleMCPServerV1(
		[]ServerSpec{configured},
		&configured,
	)
	if len(bound) != 1 || bound[0].IdentitySource != hostScheduleMCPIdentitySourceV1 ||
		bound[0].ExpectedServerName != hostScheduleMCPServerNameV1 ||
		bound[0].ExpectedServerVersion != hostScheduleMCPServerVersionV1 ||
		!bound[0].ReadOnlyToolNames[hostScheduleMCPListToolNameV1] ||
		!bound[0].ReadOnlyToolNames[hostScheduleMCPLegacyListToolNameV1] ||
		bound[0].ReadOnlyToolNames["gui_schedule_create"] ||
		hostScheduleLoopbackPortV1(bound[0]) != 9787 {
		t.Fatal("exact host schedule binding did not receive its bounded policy")
	}
	configured.Args[3] = "http://127.0.0.1:9788"
	if hostScheduleLoopbackPortV1(bound[0]) != 9787 {
		t.Fatal("bound schedule policy retained mutable caller input")
	}
}

func TestHostScheduleMCPBindingMatchesLoadedImmutableConfig(t *testing.T) {
	configured, err := LoadMCPJSONDocument([]byte(`{
		"capabilities":{"mcp":{"servers":{"gui_schedule":{
			"enabled":true,
			"transport":"stdio",
			"command":"/Applications/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper",
			"args":[
				"/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js",
				"--gui-schedule-mcp-server",
				"--base-url",
				"http://127.0.0.1:9787"
			],
			"env":{"ELECTRON_RUN_AS_NODE":"1"},
			"trustScope":"user",
			"timeoutMs":5000
		}}}}
	}`), "/workspace")
	if err != nil || len(configured) != 1 {
		t.Fatalf("load immutable schedule config: count=%d err=%v", len(configured), err)
	}
	binding := hostScheduleMCPBindingFixtureV1()
	bound := BindHostScheduleMCPServerV1(configured, &binding)
	if len(bound) != 1 || hostScheduleLoopbackPortV1(bound[0]) != 9787 ||
		!bound[0].ReadOnlyToolNames[hostScheduleMCPListToolNameV1] {
		t.Fatal("loaded immutable schedule config did not match its private host projection")
	}
}

func TestHostScheduleMCPBindingMatchesExactPrivateSecretWithoutTrustingConfigPolicy(t *testing.T) {
	configured := hostScheduleMCPBindingFixtureV1()
	configured.Args = append(configured.Args, "--secret", "private-secret")
	binding := hostScheduleMCPBindingFixtureV1()
	binding.Args = append(binding.Args, "--secret", "private-secret")

	bound := BindHostScheduleMCPServerV1([]ServerSpec{configured}, &binding)
	if len(bound) != 1 || hostScheduleLoopbackPortV1(bound[0]) != 9787 ||
		!bound[0].ReadOnlyToolNames[hostScheduleMCPListToolNameV1] ||
		!bound[0].ReadOnlyToolNames[hostScheduleMCPLegacyListToolNameV1] {
		t.Fatal("secret-bearing host schedule binding did not acquire exact read-only policy")
	}

	binding.Args[5] = "other-secret"
	drifted := BindHostScheduleMCPServerV1([]ServerSpec{configured}, &binding)
	if len(drifted) != 1 || hostScheduleLoopbackPortV1(drifted[0]) != 0 ||
		len(drifted[0].ReadOnlyToolNames) != 0 {
		t.Fatal("drifted private schedule secret acquired host policy")
	}
}

func TestHostScheduleMCPBindingRejectsConfigDriftAndUnboundServers(t *testing.T) {
	configured := hostScheduleMCPBindingFixtureV1()
	tests := map[string]func(*ServerSpec){
		"command":       func(spec *ServerSpec) { spec.Command = "/Applications/Other.app/other" },
		"entrypoint":    func(spec *ServerSpec) { spec.Args[0] = "/Applications/Other.app/server.js" },
		"loopback port": func(spec *ServerSpec) { spec.Args[3] = "http://127.0.0.1:9788" },
		"environment":   func(spec *ServerSpec) { spec.Env["EXTRA"] = "1" },
		"timeout":       func(spec *ServerSpec) { spec.TimeoutMS++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			binding := hostScheduleMCPBindingFixtureV1()
			mutate(&binding)
			bound := BindHostScheduleMCPServerV1([]ServerSpec{configured}, &binding)
			if len(bound) != 1 || bound[0].IdentitySource != "" ||
				len(bound[0].ReadOnlyToolNames) != 0 || hostScheduleLoopbackPortV1(bound[0]) != 0 {
				t.Fatal("drifted host schedule projection acquired policy")
			}
		})
	}

	unbound := BindHostScheduleMCPServerV1([]ServerSpec{configured}, nil)
	if len(unbound) != 1 || len(unbound[0].ReadOnlyToolNames) != 0 ||
		hostScheduleLoopbackPortV1(unbound[0]) != 0 {
		t.Fatal("ordinary configured schedule server acquired host policy")
	}
	duplicate := BindHostScheduleMCPServerV1(
		[]ServerSpec{configured, configured},
		&configured,
	)
	if len(duplicate) != 2 || hostScheduleLoopbackPortV1(duplicate[0]) != 0 ||
		hostScheduleLoopbackPortV1(duplicate[1]) != 0 {
		t.Fatal("ambiguous schedule config acquired host policy")
	}
	forged := hostScheduleMCPBindingFixtureV1()
	forged.ExpectedServerName = hostScheduleMCPServerNameV1
	forged.ExpectedServerVersion = hostScheduleMCPServerVersionV1
	forged.IdentitySource = hostScheduleMCPIdentitySourceV1
	forged.ReadOnlyToolNames = map[string]bool{
		hostScheduleMCPListToolNameV1:       true,
		hostScheduleMCPLegacyListToolNameV1: true,
	}
	forgedResult := BindHostScheduleMCPServerV1(
		[]ServerSpec{forged},
		&configured,
	)
	if len(forgedResult) != 1 || hostScheduleLoopbackPortV1(forgedResult[0]) != 0 {
		t.Fatal("externally configured schedule policy was mistaken for host authority")
	}
}

func TestValidateHostScheduleMCPBindingRejectsNonCanonicalDeputies(t *testing.T) {
	tests := map[string]string{
		"localhost alias": "http://localhost:9787",
		"other host":      "http://127.0.0.2:9787",
		"path":            "http://127.0.0.1:9787/schedule",
		"query":           "http://127.0.0.1:9787?next=1",
		"leading port":    "http://127.0.0.1:09787",
	}
	for name, baseURL := range tests {
		t.Run(name, func(t *testing.T) {
			spec := hostScheduleMCPBindingFixtureV1()
			spec.Args[3] = baseURL
			if _, err := ValidateHostScheduleMCPBindingV1(spec); err == nil {
				t.Fatal("non-canonical schedule deputy was accepted")
			}
		})
	}
}

func TestValidateHostScheduleMCPBindingMatchesECMAScriptTrimBoundary(t *testing.T) {
	withNEL := hostScheduleMCPBindingFixtureV1()
	withNEL.Args = append(withNEL.Args, "--secret", "\u0085private\u0085")
	if _, err := ValidateHostScheduleMCPBindingV1(withNEL); err != nil {
		t.Fatalf("ECMAScript-preserved NEL secret was rejected: %v", err)
	}

	withBOM := hostScheduleMCPBindingFixtureV1()
	withBOM.Args = append(withBOM.Args, "--secret", "\ufeffprivate\ufeff")
	if _, err := ValidateHostScheduleMCPBindingV1(withBOM); err == nil {
		t.Fatal("ECMAScript-trimmed BOM boundary was accepted")
	}

	for name, mutate := range map[string]func(*ServerSpec){
		"command separator":    func(spec *ServerSpec) { spec.Command += "/" },
		"entrypoint separator": func(spec *ServerSpec) { spec.Args[0] += "/" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := hostScheduleMCPBindingFixtureV1()
			mutate(&spec)
			if _, err := ValidateHostScheduleMCPBindingV1(spec); err == nil {
				t.Fatal("trailing path separator was accepted")
			}
		})
	}
}

func hostScheduleMCPBindingFixtureV1() ServerSpec {
	return ServerSpec{
		ID:        hostScheduleMCPServerIDV1,
		Transport: "stdio",
		Command:   "/Applications/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper",
		Args: []string{
			"/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js",
			"--gui-schedule-mcp-server",
			"--base-url",
			"http://127.0.0.1:9787",
		},
		Env:        map[string]string{"ELECTRON_RUN_AS_NODE": "1"},
		TrustScope: "user",
		TimeoutMS:  hostScheduleMCPTimeoutMSV1,
	}
}
