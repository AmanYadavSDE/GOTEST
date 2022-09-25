package bot

import "fmt"

func ParityChecker(num int) {
	if num%2 == 0 {
		fmt.Printf("Number is even")
	} else {
		fmt.Printf("Number is odd")
	}

}
