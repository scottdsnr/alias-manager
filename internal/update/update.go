// Package update checks GitHub releases for a newer alias-manager and
// replaces the running binary in place.
package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "scottdsnr/alias-manager"

// Release is the subset of the GitHub release API we care about.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var client = &http.Client{Timeout: 30 * time.Second}

// Latest fetches the most recent published release.
func Latest() (*Release, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// AssetName is the release asset for the platform we are running on.
func AssetName() string {
	return fmt.Sprintf("alias-manager_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

// Newer reports whether remote is a higher semver than local. Unparsable
// versions (a dev build, say) count as older so an update is offered.
func Newer(local, remote string) bool {
	return compare(parse(remote), parse(local)) > 0
}

func parse(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, part := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{-1, -1, -1}
		}
		out[i] = n
	}
	return out
}

func compare(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		}
	}
	return 0
}

// Apply downloads the release asset for this platform and replaces the
// running executable with it.
func Apply(rel *Release) error {
	want := AssetName()
	var url string
	for _, a := range rel.Assets {
		if a.Name == want {
			url = a.URL
			break
		}
	}
	if url == "" {
		return fmt.Errorf("release %s has no asset %s", rel.TagName, want)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	bin, err := binaryFromTarGz(resp.Body)
	if err != nil {
		return err
	}
	defer os.Remove(bin)

	// Rename-in-place: the old binary is moved aside so a running process
	// keeps working, then the new one takes its name.
	old := exe + ".old"
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	if err := copyFile(bin, exe, 0o755); err != nil {
		os.Rename(old, exe)
		return err
	}
	os.Remove(old)
	return nil
}

// binaryFromTarGz extracts the alias-manager entry to a temp file and
// returns its path.
func binaryFromTarGz(r io.Reader) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("archive contains no alias-manager binary")
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "alias-manager" {
			continue
		}
		tmp, err := os.CreateTemp("", "alias-manager-*")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(tmp, tr); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", err
		}
		tmp.Close()
		return tmp.Name(), os.Chmod(tmp.Name(), 0o755)
	}
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
