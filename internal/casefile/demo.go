package casefile

func DemoCase() Case {
	return Case{
		ID: "NS-04", Name: "NORTHSTAR", Status: "active", Owner: "ANALYST_07",
		UpdatedAt: "2026-08-10T12:48:09+05:00", SourceCount: 14,
		Tags: []string{"identity", "infrastructure", "crypto"},
		Subject: Subject{
			Codename: "SUBJECT_021", DisplayName: "N. Archer", Risk: "medium", Confidence: 87,
			Location: "Yekaterinburg / RU", LastSeen: "2026-08-09 22:17 UTC+5",
			Aliases: []string{"night_archer", "archer1977", "n.ark", "na_k472"},
			Identifiers: []Identifier{
				{Type: "email", Value: "n.archer@proton.me"},
				{Type: "domain", Value: "polar-echo.net"},
				{Type: "wallet", Value: "bc1q...4k9f"},
			},
			Relations: []Relation{
				{Name: "DOMAIN_X", Type: "infrastructure", Risk: "high"},
				{Name: "WALLET_9F", Type: "financial", Risk: "medium"},
				{Name: "PERSON_14", Type: "identity", Risk: "low"},
			},
		},
		Events: []Event{
			{ID: "EV-108", Date: "10 AUG", Time: "12:41", Type: "infrastructure", Title: "DNS record changed", Summary: "polar-echo.net resolved to a new VPS in Helsinki. The certificate fingerprint overlaps with DOMAIN_X.", Source: "Passive DNS", SourceURL: "pdns://polar-echo.net", Confidence: 92, Status: "verified", Fingerprint: "SHA256:8A7C...21F0", Indicators: []string{"185.246.87.42", "AS202053", "polar-echo.net"}, Notes: []Note{{Text: "Certificate overlap confirmed against two independent sources.", CreatedAt: "12:46"}}},
			{ID: "EV-107", Date: "10 AUG", Time: "11:36", Type: "media", Title: "EXIF location recovered", Summary: "An archived profile image retained GPS coordinates within 600 m of the subject's recurring location.", Source: "Archive capture", SourceURL: "archive://img/0841", Confidence: 81, Status: "pending", Fingerprint: "SHA256:F922...A18B", Indicators: []string{"56.8389N", "60.6057E", "IMG_0841.jpg"}},
			{ID: "EV-106", Date: "10 AUG", Time: "10:02", Type: "identity", Title: "Alias intersection found", Summary: "night_archer and na_k472 share recovery fragments, timezone patterns, and avatar history.", Source: "Identity graph", SourceURL: "graph://alias/021", Confidence: 88, Status: "verified", Fingerprint: "CASELINK:7E2B-11", Indicators: []string{"night_archer", "na_k472", "UTC+5"}},
			{ID: "EV-105", Date: "10 AUG", Time: "09:14", Type: "infrastructure", Title: "Domain registration indexed", Summary: "A privacy-protected registration used a nameserver previously connected to three archived handles.", Source: "WHOIS history", SourceURL: "whois://polar-echo.net", Confidence: 73, Status: "pending", Fingerprint: "SHA256:12BC...90D4", Indicators: []string{"ns1.kauri.host", "2024-11-18"}},
			{ID: "EV-104", Date: "09 AUG", Time: "22:17", Type: "social", Title: "Handle mentioned in channel", Summary: "A public channel referenced the primary alias alongside a wallet fragment and a delivery window.", Source: "Public channel", SourceURL: "source://channel/7741", Confidence: 64, Status: "pending", Fingerprint: "MSG:8841-22", Indicators: []string{"@night_archer", "4k9f", "UTC+5"}},
			{ID: "EV-103", Date: "09 AUG", Time: "18:32", Type: "financial", Title: "Wallet cluster expanded", Summary: "Two low-volume addresses converged on WALLET_9F through a common exchange deposit route.", Source: "Chain analysis", SourceURL: "chain://cluster/9f", Confidence: 79, Status: "verified", Fingerprint: "TX:19FD...871A", Indicators: []string{"WALLET_9F", "bc1q...4k9f", "0.184 BTC"}},
		},
	}
}
