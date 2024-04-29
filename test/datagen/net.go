package datagen

import "fmt"

func RandHostPort() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d",
		RandIntN(254),
		RandIntN(254),
		RandIntN(254),
		RandIntN(254),
		RandIntN(10000))
}
