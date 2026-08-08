package pluginhost

import (
	"strconv"
	"strings"
)

func ParseSemanticVersion(value string) ([]int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return nil, false
	}
	parsed := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil, false
		}
		parsed[index] = value
	}
	return parsed, true
}

func CompareSemanticVersion(left, right string) int {
	leftParts, leftOK := ParseSemanticVersion(left)
	rightParts, rightOK := ParseSemanticVersion(right)
	if !leftOK || !rightOK {
		return 0
	}
	for index := 0; index < 3; index++ {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}
