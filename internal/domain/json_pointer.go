package domain

import "strings"

func ValidJSONPointer(pointer string) bool {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return false
	}
	for index := 0; index < len(pointer); index++ {
		if pointer[index] == '~' && (index+1 >= len(pointer) || (pointer[index+1] != '0' && pointer[index+1] != '1')) {
			return false
		}
	}
	return true
}
