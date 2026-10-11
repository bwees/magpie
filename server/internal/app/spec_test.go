package app

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"

	"magpie/internal/platform/owner"
)

const (
	contextMapPath          = "../../../docs/spec/00-context-map.md"
	eventCatalogHeading     = "## Event catalog"
	queryDependencyHeading  = "## Query dependencies"
	sectionPrefix           = "## "
	codeFence               = "```"
	tableRowPrefix          = "| `"
	tableCellSeparator      = "|"
	ephemeralMarker         = "(ephemeral)"
	messageTypeSeparator    = "."
	commandPayloadStart     = "{"
	dependsOnArrow          = "→"
	noDependencies          = "(none)"
	dependencyListSeparator = ","
	dependencyNoteStart     = "("
)

var (
	backtickedName = regexp.MustCompile("`([^`]+)`")
	dependencyLine = regexp.MustCompile(`^(\w+)\s+([→←])\s+(.*)$`)
)

// specSection returns the lines under heading up to the next section heading.
func specSection(t *testing.T, heading string) []string {
	t.Helper()
	file, err := os.Open(contextMapPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var lines []string
	inSection := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, sectionPrefix) {
			inSection = strings.HasPrefix(line, heading)
			continue
		}
		if inSection {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Fatalf("no %q section in %s", heading, contextMapPath)
	}
	return lines
}

// catalogMessages reads the event and command tables of the context map: message type → durable.
// A cell like "`transcode.AgentOnline` / `AgentOffline`" names two events of one context.
func catalogMessages(t *testing.T) map[string]bool {
	t.Helper()
	messages := map[string]bool{}
	for _, line := range specSection(t, eventCatalogHeading) {
		if !strings.HasPrefix(line, tableRowPrefix) {
			continue
		}
		cell := strings.Split(line, tableCellSeparator)[1]
		durable := !strings.Contains(cell, ephemeralMarker)
		contextPrefix := ""
		for _, match := range backtickedName.FindAllStringSubmatch(cell, -1) {
			name, _, _ := strings.Cut(match[1], commandPayloadStart)
			if before, after, found := strings.Cut(name, messageTypeSeparator); found {
				contextPrefix, name = before, after
			}
			messages[contextPrefix+messageTypeSeparator+name] = durable
		}
	}
	return messages
}

// queryDependencies reads the acyclic Reader graph of the context map.
func queryDependencies(t *testing.T) map[owner.Name][]owner.Name {
	t.Helper()
	graph := map[owner.Name][]owner.Name{}
	for _, line := range specSection(t, queryDependencyHeading) {
		match := dependencyLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || strings.HasPrefix(line, codeFence) {
			continue
		}
		from := owner.Name(strings.ToLower(match[1]))
		graph[from] = nil
		targets, _, _ := strings.Cut(match[3], dependencyNoteStart)
		if match[2] != dependsOnArrow || strings.TrimSpace(match[3]) == noDependencies {
			continue
		}
		for target := range strings.SplitSeq(targets, dependencyListSeparator) {
			graph[from] = append(graph[from], owner.Name(strings.ToLower(strings.TrimSpace(target))))
		}
	}
	return graph
}
