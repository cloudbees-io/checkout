package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

// Implement the sort.Interface for []Version based on Major, Minor, Patch
type ByVersion []Version

func (a ByVersion) Len() int      { return len(a) }
func (a ByVersion) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByVersion) Less(i, j int) bool {
	if a[i].Major != a[j].Major {
		return a[i].Major < a[j].Major
	}
	if a[i].Minor != a[j].Minor {
		return a[i].Minor < a[j].Minor
	}
	return a[i].Patch < a[j].Patch
}

func main() {
	cmd := exec.Command("git", "ls-remote", "--tags", "--refs")
	output, err := cmd.Output()
	if err != nil {
		fmt.Errorf("Failed to execute git command:", err)
		os.Exit(1)
	}

	re := regexp.MustCompile(`v(\d+)\.(\d+)\.(\d+)`)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var versions []Version

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) > 1 {
			tag := parts[len(parts)-1]
			tagParts := strings.Split(tag, "/")
			if len(tagParts) > 1 {
				versionString := tagParts[len(tagParts)-1]
				matches := re.FindStringSubmatch(versionString)
				if matches != nil {
					// Convert strings to integers
					major, _ := strconv.Atoi(matches[1])
					minor, _ := strconv.Atoi(matches[2])
					patch, _ := strconv.Atoi(matches[3])
					versions = append(versions, Version{major, minor, patch})
				}
			}
		}
	}

	sort.Sort(ByVersion(versions))

	if len(versions) > 0 {
		highest := versions[len(versions)-1]
		bump := strings.ToLower(os.Getenv("BUMP"))
		for _, v := range os.Args {
			switch strings.ToLower(v) {
			case "--major":
				bump = "major"
			case "--minor":
				bump = "minor"
			}
		}
		switch bump {
		case "major":
			fmt.Printf("v%d.0.0\n", highest.Major+1)
		case "minor":
			fmt.Printf("v%d.%d.0\n", highest.Major, highest.Minor+1)
		default:
			fmt.Printf("v%d.%d.%d\n", highest.Major, highest.Minor, highest.Patch+1)
		}
	} else {
		fmt.Println("v0.0.1")
	}
}
