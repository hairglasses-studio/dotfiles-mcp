package enhancer

import (
	"fmt"
	"regexp"
	"strings"
)

// Example detection patterns for bare examples in text
var (
	// "Example:" or "Example 1:" headers
	exampleHeaderPattern = regexp.MustCompile(`(?mi)^#{0,3}\s*example\s*\d*\s*:`)
	// Input/Output pairs
	inputOutputPattern = regexp.MustCompile(`(?mi)^(input|output|query|answer|question|response|prompt|result)\s*:\s*`)
	// Arrow-separated transformations: "foo -> bar"
	arrowPattern = regexp.MustCompile(`(?m)^.{5,}\s*(->|=>|→)\s*.{5,}$`)
	// "For example:" followed by content
	forExamplePattern = regexp.MustCompile(`(?mi)for example[,:]`)
)

// exampleBlock represents a detected example in the text
type exampleBlock struct {
	Start int
	End   int
	Text  string
}

// DetectAndWrapExamples finds bare examples in text and wraps them in <example> tags.
// Returns the modified text and a list of improvements, or the original text unchanged.
func DetectAndWrapExamples(text string) (string, []string) {
	// Already has example tags — don't double-wrap
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<example") {
		return text, nil
	}

	// Strategy 1: Detect Input/Output pairs
	if wrapped, imps := wrapInputOutputPairs(text); imps != nil {
		return wrapped, imps
	}

	// Strategy 2: Detect "Example N:" sections
	if wrapped, imps := wrapExampleHeaders(text); imps != nil {
		return wrapped, imps
	}

	// Strategy 3: Detect arrow transformations (A -> B)
	if wrapped, imps := wrapArrowExamples(text); imps != nil {
		return wrapped, imps
	}

	return text, nil
}

// wrapInputOutputPairs detects Input:/Output: pairs and wraps them
func wrapInputOutputPairs(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	type pair struct {
		inputLine  int
		outputLine int
	}
	var pairs []pair

	for i := 0; i < len(lines)-1; i++ {
		lineLower := strings.ToLower(strings.TrimSpace(lines[i]))
		nextLower := strings.ToLower(strings.TrimSpace(lines[i+1]))

		isInput := strings.HasPrefix(lineLower, "input:") || strings.HasPrefix(lineLower, "query:") ||
			strings.HasPrefix(lineLower, "question:") || strings.HasPrefix(lineLower, "prompt:")
		isOutput := strings.HasPrefix(nextLower, "output:") || strings.HasPrefix(nextLower, "answer:") ||
			strings.HasPrefix(nextLower, "response:") || strings.HasPrefix(nextLower, "result:")

		if isInput && isOutput {
			pairs = append(pairs, pair{inputLine: i, outputLine: i + 1})
			i++ // skip the output line
		}
	}

	if len(pairs) < 2 {
		return text, nil // need at least 2 pairs to wrap
	}

	// Wrap each pair in <example> tags, wrap all in <examples>
	var b strings.Builder
	lastEnd := 0

	b.WriteString(lines[0]) // write lines before first pair
	for i := 1; i < pairs[0].inputLine; i++ {
		b.WriteString("\n")
		b.WriteString(lines[i])
	}
	if pairs[0].inputLine > 0 {
		b.WriteString("\n")
	}

	b.WriteString("\n<examples>\n")
	for idx, p := range pairs {
		// Write any lines between pairs
		if idx > 0 {
			for i := lastEnd + 1; i < p.inputLine; i++ {
				// Skip blank lines between examples
				if strings.TrimSpace(lines[i]) != "" {
					b.WriteString(lines[i])
					b.WriteString("\n")
				}
			}
		}

		fmt.Fprintf(&b, "<example index=\"%d\">\n", idx+1)
		b.WriteString(lines[p.inputLine])
		b.WriteString("\n")
		b.WriteString(lines[p.outputLine])
		b.WriteString("\n")
		b.WriteString("</example>\n")
		lastEnd = p.outputLine
	}
	b.WriteString("</examples>")

	// Write remaining lines after last pair
	for i := lastEnd + 1; i < len(lines); i++ {
		b.WriteString("\n")
		b.WriteString(lines[i])
	}

	return b.String(), []string{fmt.Sprintf("Wrapped %d input/output pairs in <examples><example> tags", len(pairs))}
}

// wrapExampleHeaders detects "Example N:" sections and wraps them
func wrapExampleHeaders(text string) (string, []string) {
	locs := exampleHeaderPattern.FindAllStringIndex(text, -1)
	if len(locs) < 2 {
		return text, nil // need at least 2 examples
	}

	// Determine each example's boundary (from header to next header or end)
	type section struct {
		start int
		end   int
	}
	var sections []section
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		sections = append(sections, section{start: loc[0], end: end})
	}

	// Build output: text before first example, then wrapped examples, then rest
	var b strings.Builder
	b.WriteString(text[:sections[0].start])
	b.WriteString("\n<examples>\n")
	for i, s := range sections {
		fmt.Fprintf(&b, "<example index=\"%d\">\n", i+1)
		b.WriteString(strings.TrimSpace(text[s.start:s.end]))
		b.WriteString("\n</example>\n")
	}
	b.WriteString("</examples>\n")

	return b.String(), []string{fmt.Sprintf("Wrapped %d example sections in <examples><example> tags", len(sections))}
}

// wrapArrowExamples detects "A -> B" transformation patterns and wraps them
func wrapArrowExamples(text string) (string, []string) {
	locs := arrowPattern.FindAllStringIndex(text, -1)
	if len(locs) < 2 {
		return text, nil
	}

	// Build with wrapped examples
	var b strings.Builder
	lastEnd := 0

	b.WriteString(text[:locs[0][0]])
	b.WriteString("\n<examples>\n")
	for i, loc := range locs {
		// Write text between examples
		if i > 0 && lastEnd < loc[0] {
			between := strings.TrimSpace(text[lastEnd:loc[0]])
			if between != "" {
				b.WriteString(between)
				b.WriteString("\n")
			}
		}
		fmt.Fprintf(&b, "<example index=\"%d\">\n", i+1)
		b.WriteString(strings.TrimSpace(text[loc[0]:loc[1]]))
		b.WriteString("\n</example>\n")
		lastEnd = loc[1]
	}
	b.WriteString("</examples>")

	// Write remaining text
	if lastEnd < len(text) {
		b.WriteString(text[lastEnd:])
	}

	return b.String(), []string{fmt.Sprintf("Wrapped %d transformation examples (A→B) in <examples><example> tags", len(locs))}
}
