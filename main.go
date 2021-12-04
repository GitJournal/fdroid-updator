package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"strconv"

	"github.com/google/go-github/v40/github"
	"golang.org/x/oauth2"
)

const owner = "GitJournal"
const repoName = "GitJournal"
const artifactName = "APK"
const artifactsDir = "./repo"
const processedArtifactsFile = "processed_artifacts.json"

func main() {
	token := flag.String("token", "", "GitHub Access Token")
	flag.Parse()

	if token == nil || len(*token) == 0 {
		val := os.Getenv("GITHUB_TOKEN")
		if len(val) == 0 {
			log.Fatal(("Missing GitHub Access Token"))
		}
		token = &val
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	artifacts, _, err := client.Actions.ListArtifacts(ctx, owner, repoName, nil)
	if err != nil {
		log.Fatal(err)
	}

	artifactsZipDir, err := ioutil.TempDir(os.TempDir(), "artifacts")
	if err != nil {
		log.Fatal("ioutil.TempDir: %w", err)
	}
	defer os.RemoveAll(artifactsZipDir)

	err = os.MkdirAll(artifactsDir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	processedArtifacts, err := readProcessedArtifacts()
	if err != nil {
		log.Fatal("readProcesssedArtifacts: %w", err)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() != artifactName {
			continue
		}

		if artifact.GetExpired() {
			continue
		}

		id := strconv.Itoa(int(artifact.GetID()))
		contains := false
		for _, aID := range processedArtifacts {
			if id == aID {
				contains = true
				break
			}
		}
		if contains {
			continue
		}

		fileName := artifactName + id + ".zip"
		fileName = path.Join(artifactsZipDir, fileName)

		_, err := os.Stat(fileName)
		if !os.IsNotExist(err) {
			continue
		}

		fmt.Println("Downloading", fileName)
		err = DownloadArtifact(ctx, client, artifact, fileName)
		if err != nil {
			log.Fatal(err)
		}

		err = Unzip(fileName, artifactsDir)
		if err != nil {
			log.Fatal(err)
		}
	}

	processedArtifacts = []string{}
	for _, artifact := range artifacts.Artifacts {
		id := strconv.Itoa(int(artifact.GetID()))
		processedArtifacts = append(processedArtifacts, id)
	}

	err = writeProcessedArtifacts(processedArtifacts)
	if err != nil {
		log.Fatal(err)
	}
}

func DownloadArtifact(ctx context.Context, client *github.Client, arifact *github.Artifact, filepath string) error {
	req, err := client.NewRequest("GET", arifact.GetArchiveDownloadURL(), nil)
	if err != nil {
		return fmt.Errorf("DownloadArtifact Build Req: %w", err)
	}

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("DownloadArtifact os create: %w", err)
	}
	defer out.Close()

	// Get the data
	resp, err := client.Do(ctx, req, out)
	if err != nil {
		return fmt.Errorf("DownloadArtifact Http Get: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

func Unzip(src, dest string) error {
	dest = filepath.Clean(dest) + string(os.PathSeparator)

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		path := filepath.Join(dest, f.Name)
		// Check for ZipSlip: https://snyk.io/research/zip-slip-vulnerability
		if !strings.HasPrefix(path, dest) {
			return fmt.Errorf("%s: illegal file path", path)
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func readProcessedArtifacts() ([]string, error) {
	var data []string

	file, err := ioutil.ReadFile(processedArtifactsFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return []string{}, err
	}

	err = json.Unmarshal(file, &data)
	if err != nil {
		return []string{}, err
	}
	return data, nil
}

func writeProcessedArtifacts(list []string) error {
	bytes, err := json.Marshal(list)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(processedArtifactsFile, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}
