package agent

import (
	"fmt"
	"strings"
)

// UnifiedDiff generates a unified diff between old and new content.
// Returns empty string if contents are identical.
func UnifiedDiff(oldContent, newContent, fileName string) string {
	if oldContent == newContent {
		return ""
	}

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Simple line-by-line diff using longest common subsequence
	hunks := computeHunks(oldLines, newLines, 3)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", fileName))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", fileName))

	for _, hunk := range hunks {
		b.WriteString(hunk)
	}

	return b.String()
}

type editOp struct {
	kind byte // ' ' context, '-' remove, '+' add
	line string
}

func computeHunks(oldLines, newLines []string, contextLines int) []string {
	// Compute edit script using simple O(NM) diff
	ops := diffLines(oldLines, newLines)
	if len(ops) == 0 {
		return nil
	}

	// Group ops into hunks with context
	var hunks []string
	var current []editOp
	oldIdx, newIdx := 0, 0
	hunkOldStart, hunkNewStart := 0, 0
	lastChangeIdx := -contextLines - 1

	for i, op := range ops {
		isChange := op.kind != ' '

		if isChange {
			// If too far from last change, flush current hunk and start new one
			if i-lastChangeIdx > contextLines*2+1 && len(current) > 0 {
				hunks = append(hunks, formatHunk(current, hunkOldStart, hunkNewStart))
				current = nil
			}

			// Add leading context if starting new hunk
			if len(current) == 0 {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				hunkOldStart = countOps(ops[:start], '-', ' ')
				hunkNewStart = countOps(ops[:start], '+', ' ')
				for j := start; j < i; j++ {
					if ops[j].kind == ' ' {
						current = append(current, ops[j])
					}
				}
			}

			lastChangeIdx = i
		}

		if len(current) > 0 || isChange {
			current = append(current, op)
		}

		switch op.kind {
		case '-':
			oldIdx++
		case '+':
			newIdx++
		default:
			oldIdx++
			newIdx++
		}

		// Trim trailing context
		if !isChange && i-lastChangeIdx > contextLines && len(current) > 0 {
			hunks = append(hunks, formatHunk(current, hunkOldStart, hunkNewStart))
			current = nil
		}
	}

	if len(current) > 0 {
		hunks = append(hunks, formatHunk(current, hunkOldStart, hunkNewStart))
	}

	// Suppress unused variable warnings
	_ = oldIdx
	_ = newIdx

	return hunks
}

func formatHunk(ops []editOp, oldStart, newStart int) string {
	oldCount, newCount := 0, 0
	for _, op := range ops {
		switch op.kind {
		case '-':
			oldCount++
		case '+':
			newCount++
		default:
			oldCount++
			newCount++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart+1, oldCount, newStart+1, newCount))
	for _, op := range ops {
		b.WriteByte(op.kind)
		b.WriteString(op.line)
		b.WriteByte('\n')
	}
	return b.String()
}

func countOps(ops []editOp, kinds ...byte) int {
	n := 0
	for _, op := range ops {
		for _, k := range kinds {
			if op.kind == k {
				n++
				break
			}
		}
	}
	return n
}

// diffLines computes a simple edit script between two line arrays
func diffLines(a, b []string) []editOp {
	m, n := len(a), len(b)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce edit ops
	var ops []editOp
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			ops = append(ops, editOp{' ', a[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			ops = append(ops, editOp{'+', b[j-1]})
			j--
		} else {
			ops = append(ops, editOp{'-', a[i-1]})
			i--
		}
	}

	// Reverse
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}

	return ops
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty line if content ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
