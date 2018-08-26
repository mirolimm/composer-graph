package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"text/template"
)

var ff = flag.String("f", "docker-compose.yml", "docker compose file to parse")

type Service struct {
	Name      string
	Image     string
	DependsOn []string
}

func main() {
	flag.Parse()
	fmt.Println("ff", *ff)
	buf, err := ioutil.ReadFile(*ff)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := parser(buf)

	for k, service := range m {
		fmt.Printf("ServiceName: %s image: %s\n", k, service.Image)
		for _, v2 := range service.DependsOn {
			fmt.Printf("DependsOn: %s \n", v2)
		}
	}
	nodes, links := serializeForGraph(m)

	type Data struct {
		Nodes string
		Links string
	}

	dataTemp := `
var nodes = {{.Nodes}}

var links = {{.Links}}
`
	wf, err := os.Create("data.js")
	if err != nil {
		panic(err)
	}
	t := template.Must(template.New("data.js").Parse(dataTemp))
	err = t.Execute(wf, Data{nodes, links})
	if err != nil {
		panic(err)
	}
	wf.Close()

	nodes = serializeForCircle(m)
	err = ioutil.WriteFile("services.json", []byte(nodes), 0600)
	if err != nil {
		panic(err)
	}

	log.Fatal(http.ListenAndServe(":8080", http.FileServer(http.Dir("."))))
}

func serializeForGraph(m map[string]Service) (nodes string, links string) {
	type Node struct {
		ID    string `json:"id"`
		Group string `json:"group"`
		Label string `json:"label"`
		Level int    `json:"level"`
	}
	nodeList := make([]Node, 0, len(m))
	for k, v := range m {
		n := Node{}
		n.ID = k
		n.Label = k
		n.Group = v.Image
		nodeList = append(nodeList, n)
	}
	data, err := json.Marshal(nodeList)
	if err != nil {
		panic(err)
	}
	nodes = string(data)

	type Link struct {
		Target   string  `json:"target"`
		Source   string  `json:"source"`
		Strength float64 `json:"strength"`
	}
	linkList := make([]Link, 0, len(m))
	for k, v := range m {
		for _, source := range v.DependsOn {
			l := Link{}
			l.Target = k
			l.Source = source
			l.Strength = 0.1
			linkList = append(linkList, l)
		}
	}
	data, err = json.Marshal(linkList)
	if err != nil {
		panic(err)
	}
	links = string(data)

	return
}

func serializeForCircle(m map[string]Service) (nodes string) {
	type Node struct {
		Name    string   `json:"name"`
		Imports []string `json:"imports"`
		Size    int      `json:"size"`
	}
	nodeList := make([]Node, 0, len(m))
	for k, v := range m {
		n := Node{}
		n.Name = k
		n.Imports = v.DependsOn
		n.Size = 1000
		nodeList = append(nodeList, n)
	}
	data, err := json.Marshal(nodeList)
	if err != nil {
		panic(err)
	}
	nodes = string(data)

	return
}

// specific for getting services names and depend_on list
// in composer file
func parser(buf []byte) map[string]Service {
	/*
	   use state machine with functions
	   getCh gets character from stream
	   checkCh syntax check
	   findStInLine checks if string is found in line
	   readList reads all elements in list
	*/
	var pos int
	// expect indent as 2 spc
	// does not handle \r
	const (
		nl   = byte('\n')
		dash = byte('-')
		spc  = byte(' ')
		cmt  = byte('#')
	)

	getCh := func() byte {
		ch := buf[pos]
		pos++
		return ch
	}

	backToLineStart := func() {
		for pos > 0 && pos < len(buf) {
			if buf[pos] == nl {
				break
			}
			pos--
		}
		pos++
	}
	skipLine := func() {
		for pos < len(buf) {
			ch := getCh()
			if ch == nl {
				break
			}
		}
	}
	// TODO maybe check with slices
	// return indent level
	// skips indents with comment symbol
	checkIndent := func() int {
		indentLevel := 0
		// prev symbol should be nl
		if pos > 0 && pos < len(buf) && buf[pos-1] != nl {
			panic("indent should be checked on new line")
		}
		var ch byte
		for pos < len(buf) {
			ch = getCh()
			if ch == spc {
				indentLevel++
			} else if ch == cmt {
				// reset
				indentLevel = 0
				skipLine()
			} else if ch > 40 {
				// go back
				pos--
				return indentLevel
			} else {
				break
			}
		}
		pos--
		return -1

	}
	// TODO do read quoted string
	readStr := func() string {
		// reads till spc/nl
		sl := []byte{}
		for {
			ch := getCh()
			if ch == spc || ch == nl {
				break
			}
			if ch > 40 {
				sl = append(sl, ch)
			}
		}
		pos--
		return string(sl)
	}

	pos = 0
	lastServiceName := ""
	services := map[string]Service{}
	// skip first line
	for {
		skipLine()
		if pos >= len(buf) {
			break
		}
		// operation on lines
		backToLineStart()
		indent := checkIndent()
		if indent == 0 {
			str := readStr()
			// test skippable top lvl commands
			if str == "volumes:" || str == "networks:" {
				for {
					skipLine()
					// skip all configs
					if checkIndent() <= 0 {
						break
					}
				}
			}
		} else if indent == 2 {
			lastServiceName = readStr()
			// remove ":" symbol
			lastServiceName = lastServiceName[0 : len(lastServiceName)-1]
			services[lastServiceName] = Service{Name: lastServiceName, Image: "", DependsOn: []string{}}
			// rollback on services names, because of skipLine

		} else if indent == 4 {
			prop := readStr()
			if prop == "image:" {
				for {
					// skip spaces
					if getCh() > 40 {
						pos--
						break
					}
				}
				serv := services[lastServiceName]
				serv.Image = readStr()
				services[lastServiceName] = serv
			}
			if prop == "depends_on:" {
				// read list of depended services
				// until indent lvl changes
				for {
					backToLineStart()
					if checkIndent() != 6 {
						skipLine()
						break
					}
					// read list
					ch := getCh()
					if ch == dash {
						getCh()
						name := readStr()
						service, ok := services[lastServiceName]
						if !ok {
							panic("something wrong pos " + strconv.Itoa(pos))
						}
						service.DependsOn = append(service.DependsOn, name)
						services[lastServiceName] = service
					}
				}
			}
		}
	}

	return services

}

func showPos(msg string, pos int, buf []byte) {
	c := make([]byte, len(buf))
	copy(c, buf)
	if pos >= 0 && pos < len(buf) {
		c[pos] = '@'
	}
	fmt.Println(msg, "pos \n", pos, string(c))
}

// returns pos of str in txt
// TODO skip comments
// TODO impl KMP algorithm
func search(str, txt []byte) int {
	if len(str) > len(txt) {
		return -1
	}
	j := 0
	for i := 0; i < len(txt); i++ {
		if j >= len(str) {
			return i - j
		}
		if txt[i] == str[j] {
			j++
		} else {
			j = 0
		}

	}

	return -1
}
