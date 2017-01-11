package tinyprompt

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pkg/term"
)

var history []string
var historyScan = -1

// PrintHistory prints the command history
func PrintHistory() {
	for _, cmd := range history {
		fmt.Printf("%v\n", cmd)
	}
}

// GetCommand reads and returns a new command from the prompt
func GetCommand() (text string) {

	fmt.Print("> ")
line_reader:
	for {
		c := getch()
		switch {
		case bytes.Equal(c, []byte{27, 91, 68}): // left
		case bytes.Equal(c, []byte{27, 91, 67}): // right
		case bytes.Equal(c, []byte{27, 91, 65}): // up
			if len(history) > 0 && historyScan > 0 {
				historyScan = historyScan - 1
				text = history[historyScan]
				fmt.Printf("\r> %s", text)
			}

		case bytes.Equal(c, []byte{27, 91, 66}): // down
			if len(history) > 0 && historyScan < len(history)-1 {
				historyScan = historyScan + 1
				text = history[historyScan]
				fmt.Printf("\r> %s", text+strings.Repeat(" ", 20)+strings.Repeat("\b", 20))
			}

		case bytes.Equal(c, []byte{13}): // enter
			fmt.Printf("\n")
			break line_reader

		case bytes.Equal(c, []byte{9}): // tab

		case bytes.Equal(c, []byte{127}): // backspace
			if len(text) > 0 {
				text = text[:len(text)-1]
				//fmt.Printf("\b ")
				fmt.Printf("\r> %v \b", text)
			}
			//fmt.Printf(chr(8) . " ";
			//fmt.Printf("\r> %v", text)

		default:
			//fmt.Printf("Key: %v", c)
			if len(c) == 1 {
				fmt.Printf("%c", c[0])
				text = text + string(c)
			}
		}
	}

	//text, _ = reader.ReadString('\n')
	//text = strings.TrimSuffix(text, "\n")

	history = append(history, text)
	historyScan = len(history)

	return
}

func getch() []byte {
	t, _ := term.Open("/dev/tty")
	term.RawMode(t)
	bytes := make([]byte, 4)
	numRead, err := t.Read(bytes)
	t.Restore()
	t.Close()
	if err != nil {
		return nil
	}
	return bytes[0:numRead]
}
