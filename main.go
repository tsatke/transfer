package main

import (
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/sftp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"github.com/spf13/afero/sftpfs"
	"golang.org/x/crypto/ssh"
)

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
		With().
		Timestamp().
		Logger()

	localFs := afero.NewOsFs()

	cfg := NewConfig(localFs)
	host := cfg.GetString("config.host")
	port := cfg.GetString("config.port")
	user := cfg.GetString("config.user")
	pass := cfg.GetString("config.pass")

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
	}
	if cfg.GetBool("config.insecure") {
		log.Warn().
			Msg("insecure flag is set, ignoring host key")
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	addr := net.JoinHostPort(host, port)
	log.Info().
		Str("address", addr).
		Msg("dial ssh")
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("dial ssh")
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("create sftp client")
	}
	defer func() { _ = client.Close() }()

	log.Info().
		Msg("dial successful")

	remoteFs := sftpfs.New(client)

	var stepSlice []Step
	if err := cfg.UnmarshalKey("transfer", &stepSlice); err != nil {
		log.Fatal().
			Err(err).
			Msg("read 'transfer' key")
	}

	executeSteps(Steps{stepSlice}, localFs, remoteFs)
}

type Steps struct {
	Steps []Step `yaml:"transfer"`
}

type Step struct {
	From      string
	To        string
	Ignore    []string
	Overwrite bool
}

func executeSteps(steps Steps, localFs, remoteFs afero.Fs) {
	log.Info().
		Int("count", len(steps.Steps)).
		Msg("executing transfer steps in parallel")

	stepCh := make(chan Step)
	wg := &sync.WaitGroup{}
	// spawn workers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(ch <-chan Step) {
			defer wg.Done()

			for step := range ch {
				executeStep(step, localFs, remoteFs)
			}
		}(stepCh)
	}
	// spawner
	for _, step := range steps.Steps {
		stepCh <- step
	}
	close(stepCh)

	wg.Wait()
	log.Info().
		Msg("done")
}

func executeStep(step Step, localFs, remoteFs afero.Fs) {
	defer func() {
		if rec := recover(); rec != nil {
			if err, ok := rec.(error); ok {
				log.Err(err).
					Str("from", step.From).
					Str("to", step.To).
					Msg("copy failed")
			} else {
				log.Error().
					Str("from", step.From).
					Str("to", step.To).
					Msgf("%v", rec)
			}
		}
	}()

	// if there is no 'to' in the step, use the 'from'
	if step.To == "" {
		step.To = step.From
	}

	remoteStat, err := remoteFs.Stat(step.From)
	if err != nil {
		panic(err)
	}

	if remoteStat.IsDir() {
		copyDir(step, localFs, remoteFs)
	} else {
		copyFile(remoteStat, step, localFs, remoteFs)
	}
}

func copyFile(remoteStat os.FileInfo, step Step, localFs, remoteFs afero.Fs) {
	log.Info().
		Str("from", step.From).
		Str("to", step.To).
		Msg("copy")

	source, err := remoteFs.Open(step.From)
	if err != nil {
		panic(err)
	}

	flags := os.O_CREATE | os.O_WRONLY
	// if the step doesn't say to overwrite, require file does not exist
	if !step.Overwrite {
		flags |= os.O_EXCL
	}

	if err := localFs.MkdirAll(filepath.Dir(step.To), 0755); err != nil {
		panic(err)
	}

	target, err := localFs.OpenFile(step.To, flags, remoteStat.Mode())
	if err != nil {
		panic(err)
	}
	_, err = io.Copy(target, source)
	if err != nil {
		panic(err)
	}
}

func copyDir(step Step, localFs, remoteFs afero.Fs) {
	if err := afero.Walk(remoteFs, step.From, func(path string, info fs.FileInfo, err error) error {
		rel, err := filepath.Rel(step.From, path)
		if err != nil {
			panic(err)
		}
		targetPath := filepath.Join(step.To, rel)

		// create directory if it is a directory
		if info.IsDir() {
			log.Info().
				Str("path", targetPath).
				Msg("mkdir")

			if err := localFs.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		} else {
			copyFile(info, Step{
				From:      path,
				To:        targetPath,
				Ignore:    step.Ignore,
				Overwrite: step.Overwrite,
			}, localFs, remoteFs)
		}

		return nil
	}); err != nil {
		panic(err)
	}
}
