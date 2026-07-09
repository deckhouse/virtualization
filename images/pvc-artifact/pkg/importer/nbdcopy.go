package importer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
	metrics "kubevirt.io/containerized-data-importer/pkg/monitoring/metrics/pvc-importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
)

const nbdcopyProgressFD = 3

var nbdcopyBinary = "nbdcopy"

// CopyNBDToDevice copies an NBD export byte-for-byte to dest using nbdcopy.
// Progress is published to kubevirt_cdi_import_progress_total when OwnerUID is set.
func CopyNBDToDevice(nbdURL, dest string) error {
	ownerUID, _ := util.ParseEnvVar(common.OwnerUID, false)

	progressR, progressW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create nbdcopy progress pipe: %w", err)
	}

	cmd := exec.CommandContext(context.Background(), nbdcopyBinary,
		fmt.Sprintf("--progress=%d", nbdcopyProgressFD),
		"--flush",
		"--allocated",
		nbdURL,
		dest,
	)
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	cmd.ExtraFiles = []*os.File{progressW}

	if err := cmd.Start(); err != nil {
		_ = progressR.Close()
		_ = progressW.Close()
		return fmt.Errorf("start nbdcopy: %w", err)
	}
	_ = progressW.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(progressR)
		for scanner.Scan() {
			reportNbdcopyProgress(scanner.Text(), ownerUID)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			klog.V(3).Infof("nbdcopy progress reader stopped: %v", scanErr)
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	_ = progressR.Close()

	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("nbdcopy failed: %w: %s", waitErr, msg)
		}
		return fmt.Errorf("nbdcopy failed: %w", waitErr)
	}

	if ownerUID != "" {
		reportNbdcopyProgress("100/100", ownerUID)
	}
	return nil
}

func reportNbdcopyProgress(line, ownerUID string) {
	if ownerUID == "" {
		return
	}

	line = strings.TrimSpace(line)
	parts := strings.Split(line, "/")
	if len(parts) != 2 || parts[1] != "100" {
		return
	}

	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || value <= 0 {
		return
	}

	progress, err := metrics.Progress(ownerUID).Get()
	if err == nil && value > progress {
		metrics.Progress(ownerUID).Add(value - progress)
	}
}
