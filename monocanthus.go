package monocanthus

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	rootUser = "root"
	procDir  = "/proc"
)

// MemChunk contains information regarding a chunk of memory and its path if present
type MemChunk struct {
	// An empty string in the path indicates that this is an anonymous block
	Path      string
	Data      []ChunkData
	TotalSize int64
}

// ChunkData contains individual memory chunks
type ChunkData struct {
	Data []byte
	Size int64
}

// MemSample contains time series data for memory samples
type MemSample struct {
	SampleTime time.Time
	Samples    map[string]int64
}

// MemMapData represents the data provided from a memory mapping
type MemMapData struct {
	Start    int64
	End      int64
	Readable bool
	Path     string
}

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

func getProcessPid(pids []int, name string) int {
	for i := 0; i < len(pids); i++ {
		fullPath := path.Join(procDir, strconv.Itoa(pids[i]))
		b, err := ioutil.ReadFile(path.Join(fullPath, "status"))
		if err != nil {
			log.Fatal(err)
		}
		n := extractProcessName(b)
		if n == name {
			return pids[i]
		}
	}
	return -1
}

func genProcPath(name string) (ppath string, err error) {
	p := getPIDList()
	pp := getProcessPid(p, name)
	if pp < 0 {
		return "", errors.New("Unabled to find pid for process name")
	}
	ppath = path.Join(procDir, strconv.Itoa(pp))
	return
}

func checkPermission() {
	out, err := exec.Command("whoami").Output()
	if err != nil {
		log.Fatal(err)
	}
	if rootUser != strings.TrimSpace(string(out)) {
		log.Fatal("Unable to operate without root")
	}
}

func addrToInt(addr string) (v int64) {
	v, err := strconv.ParseInt(addr, 16, 64)
	if err != nil {
		errFields := strings.FieldsFunc(err.Error(), func(r rune) bool {
			if r == ':' {
				return true
			}
			return false
		})
		if len(errFields) == 3 && strings.TrimSpace(errFields[2]) == "value out of range" {
			return -1
		}
		log.Fatal(err.Error())
	}
	return
}

func parseMaps(ppath string) []string {
	b, err := ioutil.ReadFile(path.Join(ppath, "maps"))
	if err != nil {
		log.Fatal(err)
	}
	res := lines(b)
	return res
}

func getMapData(line string) (out MemMapData) {
	f := strings.Fields(line)
	out.Readable = len(f) >= 2 && f[1][0] == 'r'
	if len(f) >= 6 {
		out.Path = f[5]
	} else {
		out.Path = "anonymous"
	}
	addrSplit := strings.Split(f[0], "-")
	if len(addrSplit) != 2 {
		log.Fatal("Unable to parse address space")
	}
	out.Start = addrToInt(addrSplit[0])
	out.End = addrToInt(addrSplit[1])
	return
}

func procMap(ppath string) {
	asStr := parseMaps(ppath)
	results := make(map[string]MemChunk)
	// Go over each region
	for _, v := range asStr {
		mapData := getMapData(v)
		if mapData.Readable {
			var chunk MemChunk
			if c, ok := results[mapData.Path]; ok {
				chunk = c
			} else {
				chunk = MemChunk{}
				chunk.Path = mapData.Path
				chunk.Data = make([]ChunkData, 0, 0)
				chunk.TotalSize = 0
			}
			// Prepare new ChunkData
			dat := ChunkData{}

			// Open the memory file
			m, err := os.Open(path.Join(ppath, "mem"))
			if err != nil {
				log.Fatal(err)
			}
			defer m.Close()
			// Get the start and end of the region
			chunkSize := mapData.End - mapData.Start
			dat.Size = chunkSize
			b := make([]byte, chunkSize, chunkSize)
			_, err = m.ReadAt(b, mapData.Start)
			if err != nil {
				log.Fatal(err)
			}
			dat.Data = b
			chunk.Data = append(chunk.Data, dat)
			chunk.TotalSize += chunkSize
			results[mapData.Path] = chunk
		}
	}
	overall := int64(0)
	for k, v := range results {
		fmt.Println(k, v.TotalSize)
		overall += v.TotalSize
	}
	fmt.Println("Total size", overall)
}

// PeekMemory Returns a momentary view of the processes memory at the time of the request.
func PeekMemory(pname string) {
}

// SampleMemory returns a timestamped collection of memory usage samples.
func SampleMemory(pname string) (sample MemSample) {
	checkPermission()
	ppath, err := genProcPath(pname)
	if err != nil {
		log.Fatal(err)
	}
	mapLines := parseMaps(ppath)
	sample = MemSample{
		SampleTime: time.Now(),
		Samples:    make(map[string]int64),
	}
	for _, v := range mapLines {
		mapData := getMapData(v)
		if mapData.Readable {
			sample.Samples[mapData.Path] += (mapData.End - mapData.Start)
		}
	}
	t := int64(0)
	for _, v := range sample.Samples {
		t += v
	}
	sample.Samples["Total"] = t
	return
}

func main() {
	// Check to make sure we're running as root or sudo

	p := getPIDList()
	pp := getProcessPid(p)
	if pp < 0 {
		log.Fatal("Unable to find pid for procName: " + *procName)
	}
	procPath := path.Join(procDir, strconv.Itoa(pp))

	if *peekMode {
		procMap(procPath)
	}

	takeSample(procPath, oFile)
}
