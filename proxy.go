package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"distbuild/boong/proxy/consul"
	"distbuild/boong/proxy/proto"
	"distbuild/boong/proxy/task"
	"distbuild/boong/utils"
)

//go:embed .env
var envFile string

var (
	BuildTime string
	CommitID  string
)

const (
	buildTimeout = 30 * time.Minute
)

type NormalService struct {
	Name string `json:"ServiceName"`
}

type ConsulService struct {
	Address string `json:"ServiceAddress"`
}

var (
	compileFile     string
	listenAddresses []string
	workSpacePath   string
)

var rootCmd = &cobra.Command{
	Use:     "proxy",
	Short:   "boong proxy",
	Version: BuildTime + "-" + CommitID,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		if err := loadEnvFile(envFile); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		consulService, exists := os.LookupEnv("CONSUL_SERVICE")
		if !exists {
			_, _ = fmt.Fprintln(os.Stderr, "CONSUL_SERVICE environment variable not set")
			os.Exit(1)
		}
		if err := validArgs(ctx, consulService); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		if err := run(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

// nolint:gochecknoinits
func init() {
	cobra.OnInitialize()

	rootCmd.PersistentFlags().StringVarP(&workSpacePath, "workspace-path", "w", "", "workspace path")
	rootCmd.PersistentFlags().StringVarP(&compileFile, "compile-file", "c", "", "path to compile file")

	_ = rootCmd.MarkFlagRequired("workspace-path")

	rootCmd.Root().CompletionOptions.DisableDefaultCmd = true
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func validArgs(_ context.Context, consulService string) error {
	var err error

	if !isValidIP(consulService) {
		return errors.New("invalid Ip format\n")
	}

	listenAddresses, err = consul.GetListenAddresses(consulService)
	if err != nil {
		return errors.New("failed to get worker listen address")
	}

	if len(listenAddresses) == 0 {
		return errors.New("invalid listen address")
	}

	if len(compileFile) == 0 {
		return errors.New("invalid compileFile\n")
	}

	return nil
}

func loadEnvFile(content string) error {
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		// Skip comments or empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if _, ok := os.LookupEnv(key); !ok {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func run(ctx context.Context) error {
	options := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	var clients []proto.BuildServiceClient
	var conns []*grpc.ClientConn
	var errs []error

	for _, addr := range listenAddresses {
		conn, err := grpc.NewClient(addr, options...)
		if err != nil {
			errs = append(errs, errors.Wrap(err, "failed to create grpc client for address: "+addr))
			continue
		}
		conns = append(conns, conn)
		clients = append(clients, proto.NewBuildServiceClient(conn))
	}

	if len(clients) == 0 {
		for _, err := range errs {
			fmt.Println(err)
		}
		return errors.New("failed to connect to any address")
	}

	defer func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}()

	if err := sendBuild(ctx, clients); err != nil {
		return errors.Wrap(err, "failed to send build")
	}

	return nil
}

func sendBuild(ctx context.Context, clients []proto.BuildServiceClient) error {
	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	buf, err := task.CompileDependency(workSpacePath, compileFile)
	if err != nil {
		return errors.Wrap(err, "failed to parse compile task\n")
	}

	if len(buf) == 0 {
		return errors.New("no build tasks to process")
	}

	for i, item := range buf {
		client := clients[i%len(clients)]
		stream, err := client.SendBuild(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to send client build\n")
		}

		if err := sendBuildRequest(stream, &item); err != nil {
			return errors.Wrap(err, "failed to send build request\n")
		}
		if err := receiveBuildResponse(stream, &item); err != nil {
			return errors.Wrap(err, "failed to receive build response\n")
		}
	}

	return nil
}

func sendBuildRequest(stream grpc.BidiStreamingClient[proto.BuildRequest, proto.BuildReply], build *task.BuildInfo) error {
	var files []*proto.BuildFile

	for _, item := range build.BuildFiles {
		p := filepath.Join(workSpacePath, item)
		sum, err := utils.Checksum(p)
		if err != nil {
			return errors.Wrap(err, "failed to calculate checksum\n")
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return errors.Wrap(err, "failed to read file\n")
		}
		file := &proto.BuildFile{
			FilePath: item,
			FileData: data,
			CheckSum: sum,
		}
		files = append(files, file)
	}

	id, err := createBuildID()
	if err != nil {
		return errors.Wrap(err, "failed to create build id\n")
	}

	req := &proto.BuildRequest{
		BuildID:      id,
		BuildFiles:   files,
		BuildRule:    build.BuildRule,
		BuildPath:    workSpacePath,
		BuildTargets: build.BuildTargets,
	}

	if err := stream.Send(req); err != nil {
		return errors.Wrap(err, "failed to send request\n")
	}

	if err := stream.CloseSend(); err != nil {
		return errors.Wrap(err, "failed to close stream\n")
	}

	return nil
}

func receiveBuildResponse(stream grpc.BidiStreamingClient[proto.BuildRequest, proto.BuildReply], build *task.BuildInfo) error {
	for {
		result, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to receive response\n")
		}
		for _, target := range result.GetBuildTargets() {
			_path := filepath.Join(workSpacePath, target.TargetPath)
			if _, err := os.Stat(_path); os.IsNotExist(err) {
				if _, err := os.Stat(filepath.Dir(_path)); os.IsNotExist(err) {
					if err := os.MkdirAll(filepath.Dir(_path), os.ModePerm); err != nil {
						return errors.Wrap(err, "failed to make directory\n")
					}
				}
			}
			if err := os.WriteFile(_path, target.GetTargetData(), 0755); err != nil {
				return errors.Wrap(err, "failed to write file\n")
			}
			sum, err := utils.Checksum(_path)
			if err != nil {
				return errors.Wrap(err, "failed to calculate checksum\n")
			}
			if sum != target.GetChecksum() {
				return errors.New("checksum mismatch\n")
			}
		}
	}

	return nil
}

func createBuildID() (string, error) {
	var address string

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to get interfaces\n")
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.HardwareAddr != nil {
			address = iface.HardwareAddr.String()
			break
		}
	}

	return fmt.Sprintf("%s-%d", address, time.Now().Unix()), nil
}

func getTargetPath(request []string, target string) (string, error) {
	var _path string

	goos := runtime.GOOS

	for _, item := range request {
		if goos == "windows" {
			if filepath.Ext(target) == ".exe" {
				item = filepath.Join(filepath.Dir(item), filepath.Base(item)+".exe")
			}
		}
		if filepath.Base(item) == filepath.Base(target) {
			_path = item
			break
		}
	}

	return _path, nil
}
