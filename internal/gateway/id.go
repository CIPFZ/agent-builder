package gateway

import "fmt"

func formatID(id uint64) string {
	return fmt.Sprintf("%06d", id)
}
