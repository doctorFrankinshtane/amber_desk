package casefile

func BlankCase() Case {
	return Case{
		ID: "CASE-001", Name: "UNTITLED CASE", Status: "active", Owner: "LOCAL ANALYST",
		UpdatedAt: "", SourceCount: 0, Tags: []string{}, Events: []Event{},
		Subject: Subject{
			Codename: "UNASSIGNED", DisplayName: "No subject selected", Risk: "low", Confidence: 0,
			Location: "--", LastSeen: "--", Aliases: []string{}, Identifiers: []Identifier{}, Relations: []Relation{},
		},
	}
}
