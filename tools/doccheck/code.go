package main

import "strings"

func escaped(text string, position int) bool {
	backslashes := 0
	for position--; position >= 0 && text[position] == '\\'; position-- {
		backslashes++
	}
	return backslashes%2 != 0
}

// Masking preserves every byte position and newline for accurate diagnostics.
func maskCode(source string) string {
	masked := []byte(source)
	blank := func(start, end int) {
		for i := start; i < end; i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}
	fence, offset := "", 0
	for _, line := range strings.SplitAfter(source, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		for strings.HasPrefix(trimmed, ">") {
			trimmed = strings.TrimLeft(trimmed[1:], " \t")
		}
		marker := ""
		if len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			end := 1
			for end < len(trimmed) && trimmed[end] == trimmed[0] {
				end++
			}
			if end >= 3 {
				marker = trimmed[:end]
			}
		}
		if fence != "" {
			blank(offset, offset+len(line))
			if len(marker) >= len(fence) && marker[0] == fence[0] && strings.TrimSpace(trimmed[len(marker):]) == "" {
				fence = ""
			}
		} else if marker != "" && (marker[0] != '`' || !strings.Contains(trimmed[len(marker):], "`")) {
			fence = marker
			blank(offset, offset+len(line))
		}
		offset += len(line)
	}
	text := string(masked)
	for i := 0; i < len(text); i++ {
		if strings.HasPrefix(text[i:], "<!--") {
			end := strings.Index(text[i+4:], "-->")
			if end < 0 {
				end = len(text)
			} else {
				end += i + 7
			}
			blank(i, end)
			i = end - 1
			continue
		}
		if text[i] != '`' || escaped(text, i) {
			continue
		}
		end := i + 1
		for end < len(text) && text[end] == '`' {
			end++
		}
		for cursor := end; cursor < len(text); {
			start := strings.IndexByte(text[cursor:], '`')
			if start < 0 {
				break
			}
			start += cursor
			cursor = start + 1
			for cursor < len(text) && text[cursor] == '`' {
				cursor++
			}
			if cursor-start == end-i {
				blank(i, cursor)
				end = cursor
				break
			}
		}
		i = end - 1
	}
	return string(masked)
}
