package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	cdppage "github.com/chromedp/cdproto/page"

	"go.k6.io/k6/internal/js/modules/k6/browser/log"
)

// Video manages recording of a single page's screencast frames into an MP4
// video file via ffmpeg. It also provides the JS-facing Playwright Video API:
// Path(), SaveAs(path), Delete().
type Video struct {
	session    session
	ctx        context.Context
	cancel     context.CancelFunc
	ffmpegCmd  *exec.Cmd
	ffmpegIn   io.WriteCloser
	tmpFile    *os.File
	frameCount uint64
	outputPath string
	videoSize  *RecordVideoSize
	persister  ScreenshotPersister
	done       chan struct{}
	logger     *log.Logger

	// mu protects against concurrent calls while the video is being
	// finalized (page close triggers Stop which writes the file).
	mu sync.Mutex
}

// NewVideo creates a new Video that will save the recording to outputPath via
// the provided persister. An optional videoSize constrains the captured frame
// dimensions.
func NewVideo(
	session session,
	outputPath string,
	videoSize *RecordVideoSize,
	persister ScreenshotPersister,
	logger *log.Logger,
) *Video {
	return &Video{
		session:    session,
		outputPath: outputPath,
		videoSize:  videoSize,
		persister:  persister,
		done:       make(chan struct{}),
		logger:     logger,
	}
}

// Start begins capturing screencast frames from the session and piping them
// to an ffmpeg subprocess that produces an MP4 in a temporary file.
func (v *Video) Start(ctx context.Context) error {
	v.ctx, v.cancel = context.WithCancel(ctx)

	// Create a temporary file for ffmpeg output. The final video will be
	// persisted to outputPath via the persister in Stop().
	var err error
	v.tmpFile, err = os.CreateTemp("", "k6-screencast-*.mp4")
	if err != nil {
		return fmt.Errorf("screencast: creating temp file: %w", err)
	}
	// Close the file handle; ffmpeg will open it by path.
	tmpPath := v.tmpFile.Name()
	v.tmpFile.Close()

	v.ffmpegCmd = exec.Command("ffmpeg", //nolint:gosec
		"-f", "image2pipe",
		"-vcodec", "png",
		"-i", "pipe:0",
		"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		"-y", tmpPath,
	)

	v.ffmpegIn, err = v.ffmpegCmd.StdinPipe()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("screencast: creating ffmpeg stdin pipe: %w", err)
	}

	if err := v.ffmpegCmd.Start(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("screencast: starting ffmpeg: %w", err)
	}

	// Subscribe to screencast frame events on the session.
	ch := make(chan Event)
	v.session.on(v.ctx, []string{cdproto.EventPageScreencastFrame}, ch)

	go v.handleFrames(ch)

	// Send the CDP command to start the screencast.
	action := cdppage.StartScreencast().
		WithFormat(cdppage.ScreencastFormatPng).
		WithEveryNthFrame(1)
	if v.videoSize != nil {
		action = action.WithMaxWidth(v.videoSize.Width).
			WithMaxHeight(v.videoSize.Height)
	}
	if err := action.Do(cdp.WithExecutor(v.ctx, v.session)); err != nil {
		v.cancel()
		return fmt.Errorf("screencast: starting CDP screencast: %w", err)
	}

	v.logger.Debugf("Video:Start", "recording to %s", v.outputPath)

	return nil
}

// Stop stops the screencast, closes the ffmpeg pipe, waits for ffmpeg to
// finish encoding, and persists the resulting video file.
func (v *Video) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.logger.Debugf("Video:Stop",
		"stopping, captured %d frames", atomic.LoadUint64(&v.frameCount))

	// Send the CDP command to stop the screencast.
	_ = cdppage.StopScreencast().Do(cdp.WithExecutor(v.ctx, v.session))

	// Cancel the event subscription context so the frame handler goroutine
	// exits when the channel is closed.
	v.cancel()

	// Wait for the frame handler goroutine to finish.
	<-v.done

	// Close the ffmpeg stdin pipe to signal end of input, then wait for
	// ffmpeg to finish writing the MP4.
	if v.ffmpegIn != nil {
		v.ffmpegIn.Close()
	}

	tmpPath := v.tmpFile.Name()
	defer os.Remove(tmpPath)

	if v.ffmpegCmd != nil {
		if err := v.ffmpegCmd.Wait(); err != nil {
			v.logger.Warnf("Video:Stop", "ffmpeg exited with error: %v", err)
			return
		}
	}

	// Persist the video file to the final output path.
	if err := v.persistVideo(tmpPath); err != nil {
		v.logger.Warnf("Video:Stop", "failed to persist video: %v", err)
	}

	v.logger.Debugf("Video:Stop",
		"finished, total frames: %d", atomic.LoadUint64(&v.frameCount))
}

// Path returns the file system path where the video will be saved.
func (v *Video) Path() string {
	return v.outputPath
}

// SaveAs persists a copy of the recorded video to the specified path using the
// same ScreenshotPersister that was used for the original recording. The page
// should be closed before calling this method so that the video file has been
// fully written.
func (v *Video) SaveAs(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	f, err := os.Open(v.outputPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("video saveAs: opening source %q: %w", v.outputPath, err)
	}
	defer f.Close()

	if err := v.persister.Persist(context.Background(), path, f); err != nil {
		return fmt.Errorf("video saveAs: persisting to %q: %w", path, err)
	}

	return nil
}

// Delete removes the recorded video file from disk.
func (v *Video) Delete() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if err := os.Remove(v.outputPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("video delete: removing %q: %w", v.outputPath, err)
	}

	return nil
}

// persistVideo opens the temporary MP4 file and persists it to the configured
// output path using the persister abstraction.
func (v *Video) persistVideo(tmpPath string) error {
	f, err := os.Open(tmpPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("opening temp video file: %w", err)
	}
	defer f.Close()

	if err := v.persister.Persist(context.Background(), v.outputPath, f); err != nil {
		return fmt.Errorf("persisting video to %s: %w", v.outputPath, err)
	}

	return nil
}

// handleFrames reads screencast frame events from ch, ACKs each frame via CDP,
// base64-decodes the image data, and writes the raw PNG bytes to the ffmpeg
// stdin pipe.
func (v *Video) handleFrames(ch <-chan Event) {
	defer close(v.done)

	for ev := range ch {
		frame, ok := ev.data.(*cdppage.EventScreencastFrame)
		if !ok {
			v.logger.Warnf("Video:handleFrames",
				"unexpected event data type: %T", ev.data)
			continue
		}

		atomic.AddUint64(&v.frameCount, 1)

		// ACK the frame so Chrome keeps sending new ones.
		if err := cdppage.ScreencastFrameAck(frame.SessionID).
			Do(cdp.WithExecutor(v.ctx, v.session)); err != nil {
			v.logger.Warnf("Video:handleFrames",
				"failed to ACK frame: %v", err)
		}

		data, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			v.logger.Warnf("Video:handleFrames",
				"failed to decode frame data: %v", err)
			continue
		}

		if _, err := v.ffmpegIn.Write(data); err != nil {
			v.logger.Warnf("Video:handleFrames",
				"failed to write frame to ffmpeg: %v", err)
			return
		}
	}
}
