package main

import (
	"IsaacPaperServer/Isaac"
	"fmt"
)

func main() {
	fmt.Printf("%d.%d.%d", Isaac.MAJOR_VER, Isaac.PROTOCOL_VER, Isaac.PATCH_VER)
}
