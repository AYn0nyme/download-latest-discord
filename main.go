package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"syscall"
)

// Source - https://stackoverflow.com/a/42718395
// Posted by Chris Hopkins
// Retrieved 2026-03-04, License - CC BY-SA 3.0
const (
	OS_READ = 04
	OS_WRITE = 02
	OS_EX = 01
	OS_USER_SHIFT = 6
	OS_GROUP_SHIFT = 3
	OS_OTH_SHIFT = 0

	OS_USER_R = OS_READ<<OS_USER_SHIFT
	OS_USER_W = OS_WRITE<<OS_USER_SHIFT
	OS_USER_X = OS_EX<<OS_USER_SHIFT
	OS_USER_RW = OS_USER_R | OS_USER_W
	OS_USER_RWX = OS_USER_RW | OS_USER_X

	OS_GROUP_R = OS_READ<<OS_GROUP_SHIFT
	OS_GROUP_W = OS_WRITE<<OS_GROUP_SHIFT
	OS_GROUP_X = OS_EX<<OS_GROUP_SHIFT
	OS_GROUP_RW = OS_GROUP_R | OS_GROUP_W
	OS_GROUP_RWX = OS_GROUP_RW | OS_GROUP_X

	OS_OTH_R = OS_READ<<OS_OTH_SHIFT
	OS_OTH_W = OS_WRITE<<OS_OTH_SHIFT
	OS_OTH_X = OS_EX<<OS_OTH_SHIFT
	OS_OTH_RW = OS_OTH_R | OS_OTH_W
	OS_OTH_RWX = OS_OTH_RW | OS_OTH_X

	OS_ALL_R = OS_USER_R | OS_GROUP_R | OS_OTH_R
	OS_ALL_W = OS_USER_W | OS_GROUP_W | OS_OTH_W
	OS_ALL_X = OS_USER_X | OS_GROUP_X | OS_OTH_X
	OS_ALL_RW = OS_ALL_R | OS_ALL_W
	OS_ALL_RWX = OS_ALL_RW | OS_GROUP_X
)


var EXTRACT_TO = "/opt/Discord"
var TEMP_DIR string

func main() {
	if len(os.Args) > 1 {
		EXTRACT_TO = os.Args[1]
	}
	if !isWritable(EXTRACT_TO) {
		fmt.Printf("Cannot write to %s.\n", EXTRACT_TO)
		os.Exit(1)
	}
	req, err := http.NewRequest("GET", "https://discord.com/api/download?platform=linux&format=tar.gz", nil)
	if err != nil {
		panic(err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	if res.StatusCode != 200 {
		println("HTTP Error:", res.Status)
	}

	outFile, err := os.Create("/tmp/discord.tar.gz")
	if err != nil {
		panic(err)
	}

	// Read by 64KB blocks chunks
	buf := make([]byte, 64*1024)

	var downloadedBytes int64

	for {
		n, err := res.Body.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		outFile.Write(buf[:n])
		downloadedBytes += int64(n)
		fmt.Printf("\rDownloading... %d%%", 100*downloadedBytes/res.ContentLength)
	}

	println()

	res.Body.Close()
	outFile.Close()

	outFile, err = os.Open("/tmp/discord.tar.gz")

	if err != nil {
		panic(err)
	}

	println("Downloaded discord to /tmp/discord.tar.gz")

	tarContent, err := gzip.NewReader(outFile)
	if err != nil {
		panic(err)
	}

	tarReader := tar.NewReader(tarContent)
	TEMP_DIR, err = os.MkdirTemp("/tmp", "discord-")
	if err != nil {
		panic(err)
	}

	println("Extracting /tmp/discord.tar.gz to", TEMP_DIR)

	for {
		hdr, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.Mkdir(path.Join(TEMP_DIR, hdr.Name), 0777)
			if err != nil {
				panic(err)
			}
		case tar.TypeReg:
			newFile, err := os.Create(path.Join(TEMP_DIR, hdr.Name))
			newFile.Chmod(hdr.FileInfo().Mode())
			if err != nil {
				panic(err)
			}

			if _, err = io.Copy(newFile, tarReader); err != nil {
				panic(err)
			}
			newFile.Close()

		}
	}

	os.Mkdir(EXTRACT_TO, 0755)

	entries, err := os.ReadDir(path.Join(TEMP_DIR, "Discord"))

	if err != nil {
		panic(err)
	}
	fmt.Printf("Copying files from %s/Discord to %s\n", TEMP_DIR, EXTRACT_TO)
	ReadFilesAndWrite("", entries)
	println("Downloaded Discord successfully!")

	os.RemoveAll(TEMP_DIR)
}

func ReadFilesAndWrite(RelPath string, entries []os.DirEntry) {
	for _, entry := range entries {
		if entry.IsDir() {
			if err := os.Mkdir(path.Join(EXTRACT_TO, RelPath, entry.Name()), 0755); err != nil && !errors.Is(err, os.ErrExist) {
				panic(err)
			}
			dirEntries, err := os.ReadDir(path.Join(TEMP_DIR, "Discord", RelPath, entry.Name()))
			if err != nil {
				panic(err)
			}
			ReadFilesAndWrite(path.Join(RelPath, entry.Name()), dirEntries)
		} else {
			infos, err := entry.Info()
			if err != nil {
				panic(err)
			}
			fileWriter, err := os.OpenFile(path.Join(EXTRACT_TO, RelPath, entry.Name()), os.O_CREATE|os.O_WRONLY, infos.Mode())
			if err != nil {
				panic(err)
			}

			originalExtractedFile, err := os.Open(path.Join(TEMP_DIR, "Discord", RelPath, entry.Name()))

			if err != nil {
				panic(err)
			}

			fileWriter.ReadFrom(originalExtractedFile)

			fileWriter.Close()
			originalExtractedFile.Close()
		}
	}
}

func isWritable(Entry string) bool {
	e, err := os.Stat(Entry)
	if errors.Is(err, os.ErrNotExist) {
		s := strings.Split(Entry, "/")
		return isWritable(strings.Join(s[:len(s)-1], "/"))
	}
	if err != nil {
		return false
	}
	i := e.Sys().(*syscall.Stat_t)
	if i == nil {
		return false
	}
	if (uint32(os.Getgid()) == i.Gid && (i.Mode & uint32(OS_GROUP_W)) != 0) || (uint32(os.Getuid()) == i.Uid && (i.Mode & uint32(OS_USER_W)) != 0) || (i.Mode & uint32(OS_OTH_W)) != 0 {
		return true
	}
	return false
}
