package jetsonflash

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/l4t"
)

// cacheRoot is where the kit's members are cached, or "" if there is
// nowhere to cache them.
func cacheRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "gosd", "l4t")
}

// kitInputs returns the kit's members: from the cache under cache if a
// copy matching the kit's pins is there, otherwise from o.Kit or, once the
// user has accepted NVIDIA's licence, a download. Only a download needs the
// licence accepted here: a copy passed with --kit came from NVIDIA's own
// page.
func kitInputs(kit l4t.Kit, cache string, o FlashOptions, out io.Writer) (l4t.Inputs, error) {
	if cache != "" {
		in, err := kit.Cached(cache)
		if err == nil {
			_, _ = fmt.Fprintln(out, "== using NVIDIA's L4T kit, cached in", filepath.Join(cache, kit.SHA256))
			kit.Prune(cache)
			return in, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			_, _ = fmt.Fprintf(out, "== not using the cached kit: %v\n", err)
		}
	}

	in, err := readKit(kit, o, out)
	if err != nil {
		return l4t.Inputs{}, err
	}
	if cache != "" {
		if err := kit.Cache(cache, in); err != nil {
			_, _ = fmt.Fprintf(out, "   (couldn't cache the kit's files, so the next flash will fetch it again: %v)\n", err)
		}
		kit.Prune(cache)
	}
	return in, nil
}

func readKit(kit l4t.Kit, o FlashOptions, out io.Writer) (l4t.Inputs, error) {
	if o.Kit != "" {
		f, err := os.Open(o.Kit)
		if err != nil {
			return l4t.Inputs{}, fmt.Errorf("opening the kit passed with --kit failed: %w", err)
		}
		defer func() { _ = f.Close() }()
		_, _ = fmt.Fprintf(out, "== reading NVIDIA's L4T kit from %s\n", o.Kit)
		return kit.Read(&progress{r: f, total: kit.Size, out: out})
	}

	if err := acceptLicense(kit, o, out); err != nil {
		return l4t.Inputs{}, err
	}
	_, _ = fmt.Fprintf(out, "== downloading NVIDIA's L4T kit (%d MiB) from %s\n", kit.Size>>20, kit.URL)
	body, err := kit.Open(context.Background(), http.DefaultClient)
	if err == nil {
		defer func() { _ = body.Close() }()
		var in l4t.Inputs
		if in, err = kit.Read(&progress{r: body, total: kit.Size, out: out}); err == nil {
			return in, nil
		}
	}
	return l4t.Inputs{}, fmt.Errorf("%w; jetson-flash can also read a copy you downloaded yourself, passed with --kit", err)
}

func acceptLicense(kit l4t.Kit, o FlashOptions, out io.Writer) error {
	if o.AcceptLicense {
		return nil
	}
	if o.Terminal == nil {
		return fmt.Errorf("the module's boot chain is built from NVIDIA's L4T kit, whose licence you must accept before it is downloaded; read %s and, if you accept it, run again with --accept-nvidia-license", kit.LicenseURL)
	}
	_, _ = fmt.Fprintf(out, "jetson-flash builds the module's boot chain from NVIDIA's L4T kit, which it downloads from NVIDIA. NVIDIA's licence applies:\n  %s\nDo you accept it? [y/N] ", kit.LicenseURL)
	answer, _ := bufio.NewReader(o.Terminal).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		return nil
	}
	return errors.New("NVIDIA's licence wasn't accepted, so jetson-flash downloaded nothing and left the module alone")
}

// progress passes reads through, printing a line each time another tenth
// of total has gone by.
type progress struct {
	r           io.Reader
	total, done int64
	out         io.Writer
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	before := p.done * 10 / p.total
	p.done += int64(n)
	if after := p.done * 10 / p.total; after > before && after <= 10 {
		_, _ = fmt.Fprintf(p.out, "   %d%% of %d MiB\n", after*10, p.total>>20)
	}
	return n, err
}
