package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// Moby is the type of a Moby config file
type Moby struct {
	Kernel struct {
		Image   string
		Cmdline string
	}
	Init   string
	System []MobyImage
	Daemon []MobyImage
	Files  []struct {
		Path     string
		Contents string
	}
	Outputs []struct {
		Format string
	}
}

// MobyImage is the type of an image config, based on Compose
type MobyImage struct {
	Name         string
	Image        string
	Capabilities []string
	Binds        []string
	OomScoreAdj  int64 `yaml:"oom_score_adj"`
	Command      []string
	NetworkMode  string `yaml:"network_mode"`
	Pid          string
	Ipc          string
}

const riddler = "mobylinux/riddler:7d4545d8b8ac2700971a83f12a3446a76db28c14@sha256:11b7310df6482fc38aa52b419c2ef1065d7b9207c633d47554e13aa99f6c0b72"

// NewConfig parses a config file
func NewConfig(config []byte) (*Moby, error) {
	m := Moby{}

	err := yaml.Unmarshal(config, &m)
	if err != nil {
		return &m, err
	}

	return &m, nil
}

// ConfigToRun converts a config to a series of arguments for docker run
func ConfigToRun(order int, path string, image *MobyImage) []string {
	// riddler arguments
	so := fmt.Sprintf("%03d", order)
	args := []string{"-v", "/var/run/docker.sock:/var/run/docker.sock", riddler, image.Image, "/containers/" + path + "/" + so + "-" + image.Name}
	// docker arguments
	args = append(args, "--cap-drop", "all")
	for _, cap := range image.Capabilities {
		if strings.ToUpper(cap)[0:4] == "CAP_" {
			cap = cap[4:]
		}
		args = append(args, "--cap-add", cap)
	}
	if image.OomScoreAdj != 0 {
		args = append(args, "--oom-score-adj", strconv.FormatInt(image.OomScoreAdj, 10))
	}
	if image.NetworkMode != "" {
		// TODO only "host" supported
		args = append(args, "--net="+image.NetworkMode)
	}
	if image.Pid != "" {
		// TODO only "host" supported
		args = append(args, "--pid="+image.Pid)
	}
	if image.Ipc != "" {
		// TODO only "host" supported
		args = append(args, "--ipc="+image.Ipc)
	}
	for _, bind := range image.Binds {
		args = append(args, "-v", bind)
	}
	// image
	args = append(args, image.Image)
	// command
	args = append(args, image.Command...)

	return args
}

func filesystem(m *Moby) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	for _, f := range m.Files {
		if f.Path == "" {
			return buf, errors.New("Did not specify path for file")
		}
		if f.Contents == "" {
			return buf, errors.New("Contents of file not specified")
		}
		// we need all the leading directories
		parts := strings.Split(path.Dir(f.Path), "/")
		root := ""
		for _, p := range parts {
			if p == "." || p == "/" {
				continue
			}
			if root == "" {
				root = p
			} else {
				root = root + "/" + p
			}
			hdr := &tar.Header{
				Name:     root,
				Typeflag: tar.TypeDir,
				Mode:     0700,
			}
			err := tw.WriteHeader(hdr)
			if err != nil {
				return buf, err
			}
		}
		hdr := &tar.Header{
			Name: f.Path,
			Mode: 0600,
			Size: int64(len(f.Contents)),
		}
		err := tw.WriteHeader(hdr)
		if err != nil {
			return buf, err
		}
		_, err = tw.Write([]byte(f.Contents))
		if err != nil {
			return buf, err
		}
	}
	return buf, nil
}