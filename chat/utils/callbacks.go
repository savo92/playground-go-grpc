package utils

import "fmt"

func AfterEvent(cmd fmt.Stringer) string {
	return fmt.Sprint("after_", cmd)
}
