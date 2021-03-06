package tinyprompt

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"os"

	"github.com/pkg/term"
)

var history []string
var historyScan = -1
var historyStart = 0

// LoadHistory appends current history to the given file
func LoadHistory(path string) {
	//expand tilde
	usr, _ := user.Current()
	if path[:2] == "~/" {
		path = filepath.Join(usr.HomeDir, path[2:])
	}
	path, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Malformed path\n")
		return
	}
	f, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		history = append(history, scanner.Text())
	}
	historyScan = len(history)
	historyStart = len(history)
	f.Close()
}

// SaveHistory appends current history to the given file
func SaveHistory(path string, filter []string) {
	//expand tilde
	usr, _ := user.Current()
	if path[:2] == "~/" {
		path = filepath.Join(usr.HomeDir, path[2:])
	}
	path, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Malformed path\n")
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Printf("Couldnt write to " + path + "\n")
	}
	for i := historyStart; i < len(history); i++ {
		if !contains(filter, history[i]) {
			f.WriteString(history[i] + "\n")
		}
	}
	f.Close()
}

// PrintHistory prints the command history
func PrintHistory() {
	for _, cmd := range history {
		fmt.Printf("%v\n", cmd)
	}
}

func terminalWidth() (w int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	parts := strings.Split(strings.Trim(string(out), "\n"), " ")
	w, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return
}

// GetCommand reads and returns a new command from the prompt
func GetCommand(debugMode bool) (text string) {
	fmt.Print("> ")
	var lastkey string
	var keyPos = 0
line_reader:
	for {
		c := getch()
		switch {
		case bytes.Equal(c, []byte{3}):
			fmt.Printf("\r> " + strings.Repeat(" ", len(text)) + strings.Repeat("\b", len(text)))
			text = ""
			keyPos = 0

		case bytes.Equal(c, []byte{27, 91, 68}): // left
			if keyPos > 0 {
				fmt.Printf("\b")
				keyPos--
			}

		case bytes.Equal(c, []byte{27, 91, 67}): // right
			if keyPos < len(text) {
				fmt.Printf(string(text[keyPos]))
				keyPos++
			}

		case bytes.Equal(c, []byte{27, 91, 65}): // up
			if len(history) > 0 && historyScan > 0 {
				historyScan = historyScan - 1
				fmt.Printf("\r> %s"+strings.Repeat(" ", len(text))+strings.Repeat("\b", len(text)), history[historyScan])
				text = history[historyScan]
				keyPos = len(text)
			}

		case bytes.Equal(c, []byte{27, 91, 66}): // down
			if len(history) > 0 && historyScan < len(history)-1 {
				historyScan = historyScan + 1
				text = history[historyScan]
				keyPos = len(text)
				fmt.Printf("\r> %s", text+strings.Repeat(" ", 20)+strings.Repeat("\b", 20))
			}

		case bytes.Equal(c, []byte{13}): // enter
			fmt.Printf("\n")
			break line_reader

		case bytes.Equal(c, []byte{9}): // tab
			//tab completion... path or key
			parts := strings.Split(text, " ")
			dir := parts[len(parts)-1]
			d2 := parts[len(parts)-1]

			/*//expand tilde symbol
			usr, _ := user.Current()
			if dir[:2] == "~/" {
				d2 = filepath.Join(usr.HomeDir, dir[2:])
			}*/

			if debugMode {
				fmt.Printf("Will try to autocomplete %s, converted from %s\n", d2+"*", dir)
			}

			matches, err := filepath.Glob(d2 + "*")
			if err == nil {
				if len(matches) == 1 {
					//file or string match???
					fmt.Printf(matches[0][len(dir):] + "/")
					text = text + matches[0][len(dir):] + "/"
					keyPos = len(text)
				}
				if len(matches) > 1 && lastkey == "tab" {
					// second tab press, display list of matches
					fmt.Printf("\n")
					for i := range matches {
						_, f := filepath.Split(matches[i])
						s, err := os.Stat(matches[i])
						if err == nil && s.IsDir() {
							fmt.Printf("%v/\n", f)
						} else {
							fmt.Printf("%v\n", f)
						}

					}

					fmt.Printf("> %s", text)
				}
			}
			lastkey = "tab"

		case bytes.Equal(c, []byte{127}): // backspace
			if keyPos > 0 {
				if keyPos < len(text) {
					text = text[:keyPos-1] + text[keyPos:]
					keyPos--
					fmt.Printf("\r> %v \b"+strings.Repeat("\b", len(text)-keyPos), text)
				} else {
					text = text[:len(text)-1]
					keyPos--
					fmt.Printf("\r> %v \b", text)
				}
			}

		default:
			if keyPos < len(text) {
				fmt.Printf(string(c) + text[keyPos:] + strings.Repeat("\b", len(text)-keyPos))
				text = text[:keyPos] + string(c) + text[keyPos:]
				keyPos++
				//insert mode
			} else {
				fmt.Printf("%s", c)
				keyPos++
				text = text + string(c)
			}
			//	fmt.Printf("Uncaught: %v\n", c)
			/*if len(c) == 1 {
				fmt.Printf("%c", c[0])
				text = text + string(c)
			} else {
				if debugMode {
					fmt.Printf("Uncaught: %s\n", c)
				}
			}*/
		}

	}

	//text, _ = reader.ReadString('\n')
	//text = strings.TrimSuffix(text, "\n")

	if len(text) > 0 {
		history = append(history, text)
	}
	historyScan = len(history)

	return
}

func getch() []byte {
	t, _ := term.Open("/dev/tty")
	term.RawMode(t)
	// read MAX 512 bytes
	// puts a limit on pasted text on osx
	bytes := make([]byte, 512)
	numRead, err := t.Read(bytes)
	t.Restore()
	t.Close()
	if err != nil {
		return nil
	}
	return bytes[0:numRead]
}

func contains(list interface{}, elem interface{}) bool {
	v := reflect.ValueOf(list)
	for i := 0; i < v.Len(); i++ {
		if v.Index(i).Interface() == elem {
			return true
		}
	}
	return false
}

/*
\a   U+0007 alert or bell
\b   U+0008 backspace
\f   U+000C form feed
\n   U+000A line feed or newline
\r   U+000D carriage return
\t   U+0009 horizontal tab
\v   U+000b vertical tab
\\   U+005c backslash
\'   U+0027 single quote  (valid escape only within rune literals)
\"   U+0022 double quote  (valid escape only within string literals)
*/
