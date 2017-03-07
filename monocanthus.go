package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const (
	rootUser = "root"
	procDir  = "/proc"
)

var procName = flag.String("procName", "", "Process name to monitor")

func getPIDList() (pids []int) {
	files, err := ioutil.ReadDir(procDir)
	if err != nil {
		log.Fatal("Unable to open proc dir")
	}
	for i := 0; i < len(files); i++ {
		if n, err := strconv.Atoi(files[i].Name()); err == nil && files[i].IsDir() {
			pids = append(pids, n)
		}
	}
	return
}

func lines(in []byte) (out []string) {
	l := make([]byte, 0, 0)
	for i := 0; i < len(in); i++ {
		if in[i] != '\n' {
			l = append(l, in[i])
		} else {
			out = append(out, string(l))
			l = make([]byte, 0, 0)
		}
	}
	if len(l) > 0 {
		out = append(out, string(l))
	}
	return
}

func extractProcessName(in []byte) string {
	asStr := lines(in)
	for _, v := range asStr {
		if v[:5] == "Name:" {
			return strings.TrimSpace(v[5:])
		}
	}
	return ""
}

func getProcessPid(pids []int) int {
	for i := 0; i < len(pids); i++ {
		fullPath := path.Join(procDir, strconv.Itoa(pids[i]))
		b, err := ioutil.ReadFile(path.Join(fullPath, "status"))
		if err != nil {
			log.Fatal(err)
		}
		n := extractProcessName(b)
		if n == *procName {
			return pids[i]
		}
	}
	return -1
}

func procMap(ppath string) {
	// Get the contents of the "maps" file from the process
	b, err := ioutil.ReadFile(path.Join(ppath, "maps"))
	if err != nil {
		log.Fatal(err)
	}
	// Split out into lines for each mem region
	asStr := lines(b)
	// Go over each region
	for _, v := range asStr {
		// Split the record into fields and read the permissions field
		f := strings.Fields(v)
		perms := f[1]
		// If this region of memory is readable ...
		if perms[0] == 'r' {
			// Open the memory file
			_, err := os.Open(path.Join(ppath, "mem"))
			if err != nil {
				log.Fatal(err)
			}
			// Get the start and end of the region
			rSplit := strings.Split(f[0], "-")
			if len(rSplit) != 2 {
				log.Fatal("Unable to parse address space")
			}
			start, err := strconv.ParseInt(rSplit[0], 16, 64)
			if err != nil {
				log.Fatal(err)
			}
			end, err := strconv.ParseInt(rSplit[1], 16, 64)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(start, end)
		}
	}
}

func main() {
	flag.Parse()

	// Check to make sure we're running as root or sudo
	out, err := exec.Command("whoami").Output()
	if err != nil {
		log.Fatal(err)
	}
	if rootUser != strings.TrimSpace(string(out)) {
		log.Fatal("Unable to operate without root")
	}
	p := getPIDList()
	pp := getProcessPid(p)
	if pp < 0 {
		log.Fatal("Unabled to find pid for procName: " + *procName)
	}
	procPath := path.Join(procDir, strconv.Itoa(pp))
	procMap(procPath)
}
