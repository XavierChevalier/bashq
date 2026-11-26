package report

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XavierChevalier/bashq/internal/check"
)

const (
	homepage = "https://github.com/XavierChevalier/bashq"
	rulesDoc = homepage + "/blob/main/docs/rules.md#"
)

func ruleHelp(rule string) string {
	if check.IsShellCheck(rule) {
		return "https://www.shellcheck.net/wiki/" + rule
	}
	return rulesDoc + rule
}

func ruleSummary(rule string) string {
	if check.IsShellCheck(rule) {
		return "ShellCheck " + rule
	}
	r, _ := check.Lookup(rule)
	return r.Summary
}

// usedRules lists the rules of the findings, sorted.
func usedRules(findings []check.Finding) []string {
	seen := map[string]bool{}
	var rules []string
	for _, f := range findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			rules = append(rules, f.Rule)
		}
	}
	sort.Strings(rules)
	return rules
}

// fingerprint identifies a finding across commits: rule, file and
// function, not the line, which moves with every edit above it.
func fingerprint(f check.Finding) string {
	sum := sha256.Sum256([]byte(f.Rule + "\x00" + f.Path + "\x00" + f.Function + "\x00" + f.Message))
	return hex.EncodeToString(sum[:8])
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region sarifRegion `json:"region"`
	} `json:"physicalLocation"`
	Message *sarifMessage `json:"message,omitempty"`
}

func newSarifLocation(l check.Location) sarifLocation {
	var loc sarifLocation
	loc.PhysicalLocation.ArtifactLocation.URI = filepath.ToSlash(strings.TrimPrefix(l.Path, "./"))
	loc.PhysicalLocation.Region = sarifRegion{StartLine: max(l.Line, 1), StartColumn: l.Col}
	if l.EndLine > l.Line {
		loc.PhysicalLocation.Region.EndLine = l.EndLine
	}
	return loc
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
	HelpURI          string       `json:"helpUri"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	RelatedLocations    []sarifLocation   `json:"relatedLocations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

// sarif writes SARIF 2.1.0, the format of GitHub code scanning and most
// code review tools.
func sarif(w io.Writer, r *check.Report, version string) error {
	var driver struct {
		Name           string      `json:"name"`
		Version        string      `json:"version"`
		InformationURI string      `json:"informationUri"`
		Rules          []sarifRule `json:"rules"`
	}
	driver.Name, driver.Version, driver.InformationURI = "bashq", version, homepage
	driver.Rules = []sarifRule{}
	for _, rule := range usedRules(r.New) {
		driver.Rules = append(driver.Rules, sarifRule{ID: rule, ShortDescription: sarifMessage{ruleSummary(rule)}, HelpURI: ruleHelp(rule)})
	}

	results := []sarifResult{}
	for _, f := range r.New {
		res := sarifResult{
			RuleID:              f.Rule,
			Level:               f.Severity,
			Message:             sarifMessage{f.Message},
			Locations:           []sarifLocation{newSarifLocation(f.Location)},
			PartialFingerprints: map[string]string{"bashq/v1": fingerprint(f)},
		}
		if f.Related != nil {
			loc := newSarifLocation(*f.Related)
			loc.Message = &sarifMessage{"original copy"}
			res.RelatedLocations = []sarifLocation{loc}
		}
		results = append(results, res)
	}

	type run struct {
		Tool struct {
			Driver any `json:"driver"`
		} `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	var one run
	one.Tool.Driver = driver
	one.Results = results
	return encode(w, map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs":    []run{one},
	})
}

// sonar writes SonarQube's generic issue import format, for
// sonar.externalIssuesReportPaths.
func sonar(w io.Writer, r *check.Report) error {
	type impact struct {
		SoftwareQuality string `json:"softwareQuality"`
		Severity        string `json:"severity"`
	}
	type rule struct {
		ID                 string   `json:"id"`
		Name               string   `json:"name"`
		Description        string   `json:"description"`
		EngineID           string   `json:"engineId"`
		CleanCodeAttribute string   `json:"cleanCodeAttribute"`
		Impacts            []impact `json:"impacts"`
	}
	type textRange struct {
		StartLine int `json:"startLine"`
		EndLine   int `json:"endLine,omitempty"`
	}
	type location struct {
		Message   string    `json:"message"`
		FilePath  string    `json:"filePath"`
		TextRange textRange `json:"textRange"`
	}
	type issue struct {
		RuleID             string     `json:"ruleId"`
		PrimaryLocation    location   `json:"primaryLocation"`
		SecondaryLocations []location `json:"secondaryLocations,omitempty"`
	}

	rules := []rule{}
	for _, id := range usedRules(r.New) {
		attribute, quality, severity := "FOCUSED", "MAINTAINABILITY", "MEDIUM"
		switch {
		case id == "duplication":
			attribute = "DISTINCT"
		case id == "parse-error":
			attribute, quality, severity = "LOGICAL", "RELIABILITY", "HIGH"
		case check.IsShellCheck(id):
			attribute, quality = "LOGICAL", "RELIABILITY"
		}
		rules = append(rules, rule{
			ID: id, Name: ruleSummary(id), Description: ruleHelp(id), EngineID: "bashq",
			CleanCodeAttribute: attribute, Impacts: []impact{{quality, severity}},
		})
	}
	issues := []issue{}
	for _, f := range r.New {
		end := 0
		if f.EndLine > f.Line {
			end = f.EndLine
		}
		is := issue{RuleID: f.Rule, PrimaryLocation: location{
			Message: f.Message, FilePath: f.Path, TextRange: textRange{StartLine: max(f.Line, 1), EndLine: end},
		}}
		if f.Related != nil {
			is.SecondaryLocations = []location{{
				Message: "original copy", FilePath: f.Related.Path,
				TextRange: textRange{StartLine: f.Related.Line, EndLine: f.Related.EndLine},
			}}
		}
		issues = append(issues, is)
	}
	return encode(w, map[string]any{"rules": rules, "issues": issues})
}
