package utils

import (
	"strings"
)

// WrapText wraps text to a specified width
func WrapText(text string, width int) string {
	if width <= 10 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		currentLine := words[0]
		currentWidth := len(words[0])

		for i := 1; i < len(words); i++ {
			word := words[i]
			if currentWidth+1+len(word) > width {
				result = append(result, currentLine)
				currentLine = word
				currentWidth = len(word)
			} else {
				currentLine += " " + word
				currentWidth += 1 + len(word)
			}
		}

		if currentLine != "" {
			result = append(result, currentLine)
		}
	}

	return strings.Join(result, "\n")
}
