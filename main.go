package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
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
const artifactsZipDir = "/home/vhanda/src/env/data/github-artifacts"
const artifactsDir = "/home/vhanda/src/env/data/fdroid"

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ""},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	artifacts, _, err := client.Actions.ListArtifacts(ctx, owner, repoName, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = os.MkdirAll(artifactsZipDir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	err = os.MkdirAll(artifactsDir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() != artifactName {
			continue
		}

		if artifact.GetExpired() {
			continue
		}

		fileName := artifactName + strconv.Itoa(int(artifact.GetID())) + ".zip"
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
