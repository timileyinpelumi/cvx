package ai

// profileSchema mirrors model.Profile (and its nested types) without any id
// fields — the LLM never assigns ids; model.AssignIDs does that after
// unmarshaling. Every object is closed (additionalProperties: false) and
// fully required; string fields use "" to signal an absent value rather than
// null so the schema stays free of "nullable" unions.
var profileSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"isResume":        map[string]any{"type": "boolean"},
		"notResumeReason": map[string]any{"type": "string"},
		"name":            map[string]any{"type": "string"},
		"email":           map[string]any{"type": "string"},
		"phone":           map[string]any{"type": "string"},
		"location":        map[string]any{"type": "string"},
		"summary":         map[string]any{"type": "string"},
		"links": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label": map[string]any{"type": "string"},
					"url":   map[string]any{"type": "string"},
				},
				"required":             []string{"label", "url"},
				"additionalProperties": false,
			},
		},
		"certifications": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string"},
					"issuer": map[string]any{"type": "string"},
					"year":   map[string]any{"type": "string"},
				},
				"required":             []string{"name", "issuer", "year"},
				"additionalProperties": false,
			},
		},
		"languages": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"interests": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"skills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":         map[string]any{"type": "string"},
					"title":        map[string]any{"type": "string"},
					"organization": map[string]any{"type": "string"},
					"startDate":    map[string]any{"type": "string"},
					"endDate":      map[string]any{"type": "string"},
					"bullets": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
								"skills": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
							"required":             []string{"text", "skills"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"kind", "title", "organization", "startDate", "endDate", "bullets"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"isResume", "notResumeReason", "name", "email", "phone", "location", "summary", "links", "certifications", "languages", "interests", "skills", "items"},
	"additionalProperties": false,
}

// tailoredSchema mirrors model.Tailored exactly (camelCase keys), including
// the source ids the guardrail in tailor.go checks against the profile.
var tailoredSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"targetRole":  map[string]any{"type": "string"},
		"roleSummary": map[string]any{"type": "string"},
		"headline":    map[string]any{"type": "string"},
		"summary":     map[string]any{"type": "string"},
		"selectedSkills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"certifications": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"languages":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"interests":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"sections": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
						"enum": []string{"experience", "projects", "education", "certifications", "volunteering", "other"},
					},
					"title": map[string]any{"type": "string"},
					"items": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"sourceId":     map[string]any{"type": "string"},
								"title":        map[string]any{"type": "string"},
								"organization": map[string]any{"type": "string"},
								"dates":        map[string]any{"type": "string"},
								"bullets": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"sourceBulletId": map[string]any{"type": "string"},
											"text":           map[string]any{"type": "string"},
										},
										"required":             []string{"sourceBulletId", "text"},
										"additionalProperties": false,
									},
								},
							},
							"required":             []string{"sourceId", "title", "organization", "dates", "bullets"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"kind", "title", "items"},
				"additionalProperties": false,
			},
		},
		"gaps": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"requirement": map[string]any{"type": "string"},
					"evidence":    map[string]any{"type": "string"},
					"severity":    map[string]any{"type": "string", "enum": []string{"missing", "weak"}},
				},
				"required":             []string{"requirement", "evidence", "severity"},
				"additionalProperties": false,
			},
		},
		"whatChanged": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"targetRole", "roleSummary", "headline", "summary", "selectedSkills", "certifications", "languages", "interests", "sections", "gaps", "whatChanged"},
	"additionalProperties": false,
}

// bulletDraftSchema mirrors model.BulletDraft (a bullet with no id), shared
// by both newItems' bullets and bulletAdditions' bullets below.
var bulletDraftSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"text": map[string]any{"type": "string"},
		"skills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"text", "skills"},
	"additionalProperties": false,
}

// profileAdditionsSchema mirrors model.ProfileAdditions exactly (camelCase
// keys), without any id fields — MergeAdditions assigns those deterministically
// after unmarshaling, same discipline as profileSchema.
var profileAdditionsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"useful":          map[string]any{"type": "boolean"},
		"notUsefulReason": map[string]any{"type": "string"},
		"newSkills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"newCertifications": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string"},
					"issuer": map[string]any{"type": "string"},
					"year":   map[string]any{"type": "string"},
				},
				"required":             []string{"name", "issuer", "year"},
				"additionalProperties": false,
			},
		},
		"newLanguages": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"newInterests": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"newItems": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":         map[string]any{"type": "string"},
					"title":        map[string]any{"type": "string"},
					"organization": map[string]any{"type": "string"},
					"startDate":    map[string]any{"type": "string"},
					"endDate":      map[string]any{"type": "string"},
					"bullets": map[string]any{
						"type":  "array",
						"items": bulletDraftSchema,
					},
				},
				"required":             []string{"kind", "title", "organization", "startDate", "endDate", "bullets"},
				"additionalProperties": false,
			},
		},
		"bulletAdditions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"itemId": map[string]any{"type": "string"},
					"bullets": map[string]any{
						"type":  "array",
						"items": bulletDraftSchema,
					},
				},
				"required":             []string{"itemId", "bullets"},
				"additionalProperties": false,
			},
		},
	},
	"required": []string{"useful", "notUsefulReason", "newSkills", "newCertifications",
		"newLanguages", "newInterests", "newItems", "bulletAdditions"},
	"additionalProperties": false,
}

// coverLetterSchema mirrors model.CoverLetter exactly (camelCase keys).
var coverLetterSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"paragraphs": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"closing": map[string]any{"type": "string"},
	},
	"required":             []string{"paragraphs", "closing"},
	"additionalProperties": false,
}

// recruiterEmailSchema mirrors model.RecruiterEmail exactly (camelCase keys).
var recruiterEmailSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"subject": map[string]any{"type": "string"},
		"paragraphs": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"closing": map[string]any{"type": "string"},
	},
	"required":             []string{"subject", "paragraphs", "closing"},
	"additionalProperties": false,
}
