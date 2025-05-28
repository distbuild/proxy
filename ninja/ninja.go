package ninja

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

const (
	executableName = "ninja"
)

type Ninja interface {
	Init(context.Context, string) error
	Deinit(context.Context) error
	Load(context.Context) ([]Build, error)
}

type Config struct{}

type File struct {
	Directory string `json:"directory"`
	Command   string `json:"command"`
	File      string `json:"file"`
	Output    string `json:"output"`
}

type Build struct {
	BuildLang    string
	BuildFiles   []string
	BuildRule    string
	BuildPath    string
	BuildTargets []string
}

type ninja struct {
	cfg  *Config
	file string
}

func New(_ context.Context, cfg *Config) Ninja {
	return &ninja{
		cfg: cfg,
	}
}

func DefaultConfig() *Config {
	return &Config{}
}

func (n *ninja) Init(ctx context.Context, name string) error {
	n.file = name

	if err := n.check(ctx); err != nil {
		return errors.Wrap(err, "failed to init ninja\n")
	}

	return nil
}

func (n *ninja) Deinit(_ context.Context) error {
	return nil
}

func (n *ninja) Load(ctx context.Context) ([]Build, error) {
	var buf []Build

	out, err := n.run(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run ninja\n")
	}

	buf, err = n.parse(ctx, out)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ninja\n")
	}

	return buf, nil
}

func (n *ninja) check(_ context.Context) error {
	if _, err := os.Stat(n.file); err != nil {
		return errors.New("invalid file name\n")
	}

	return nil
}

func (n *ninja) run(ctx context.Context) ([]byte, error) {
	path, err := exec.LookPath(executableName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find executable name\n")
	}

	cmd := exec.CommandContext(ctx, path, "-f", n.file, "-t", "compdb")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "failed to run command\n")
	}

	return out, nil
}

func (n *ninja) parse(_ context.Context, data []byte) ([]Build, error) {
	var buf []File
	var ret []Build

	if err := json.Unmarshal(data, &buf); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal json\n")
	}

	for _, item := range buf {
		b := Build{
			BuildLang:    "",
			BuildFiles:   []string{item.File},
			BuildRule:    item.Command,
			BuildPath:    item.Directory,
			BuildTargets: []string{item.Output},
		}
		ret = append(ret, b)
	}

	return ret, nil
}
