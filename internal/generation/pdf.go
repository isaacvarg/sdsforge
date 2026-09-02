package generation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// pdfTimeout bounds one print.
//
// A sheet of this size prints in a second or two; a minute is generous enough
// that a slow machine is never the reason a build fails, and short enough that
// a wedged browser does not hang the CLI forever.
const pdfTimeout = 60 * time.Second

// PDFOptions is the page geometry, in inches -- the unit Chrome's
// Page.printToPDF takes. config.PDF.Geometry converts to it.
type PDFOptions struct {
	PaperWidth  float64
	PaperHeight float64
	Margin      float64

	// Footer is the running footer's markup, from RenderFooter.
	Footer string
}

// RenderPDF prints the sheet with a headless Chrome and returns the PDF.
//
// The HTML is handed to the browser through Page.setDocumentContent rather than
// written to a temp file and navigated to: every image in a generated sheet is
// already a data: URI, so there is no base URL to resolve and nothing on disk
// to clean up if the browser dies mid-print.
func RenderPDF(ctx context.Context, browserPath string, html []byte, opts PDFOptions) ([]byte, error) {
	// Kept so an interrupt can be told apart from a browser fault below.
	interrupted := ctx

	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browserPath))...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	var pdf []byte
	runErr := chromedp.Run(browserCtx,
		// about:blank first: setDocumentContent needs a frame to replace, and
		// there is no frame until something has been navigated to.
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("reading the page's frame tree: %w", err)
			}
			return page.SetDocumentContent(tree.Frame.ID, string(html)).Do(ctx)
		}),
		waitPrintable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			out, _, err := page.PrintToPDF().
				WithPaperWidth(opts.PaperWidth).
				WithPaperHeight(opts.PaperHeight).
				WithMarginTop(opts.Margin).
				WithMarginBottom(opts.Margin).
				WithMarginLeft(opts.Margin).
				WithMarginRight(opts.Margin).
				// The stylesheet bands its section headings; without this they
				// print as bare text on white.
				WithPrintBackground(true).
				// The geometry above wins over the stylesheet's @page rule, so
				// the footer's padding and the page margin cannot disagree.
				WithPreferCSSPageSize(false).
				WithDisplayHeaderFooter(true).
				// An empty header is not the same as no header: leaving this
				// out while displayHeaderFooter is on gives Chrome's own
				// title-and-date header.
				WithHeaderTemplate("<div></div>").
				WithFooterTemplate(opts.Footer).
				Do(ctx)
			if err != nil {
				return fmt.Errorf("printing to PDF: %w", err)
			}
			pdf = out
			return nil
		}),
	)
	if runErr != nil {
		// Cancelling the context kills the browser, and chromedp reports
		// whatever it was doing when it died -- which reads as a browser fault
		// rather than the Ctrl-C or the timeout it actually was.
		if err := interrupted.Err(); err != nil {
			return nil, err
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: gave up printing after %s: %w",
				browserPath, pdfTimeout, runErr)
		}
		return nil, fmt.Errorf("%s: %w", browserPath, runErr)
	}
	return pdf, nil
}

// waitPrintable blocks until the page has everything it needs to lay out.
//
// The sheet's artwork is embedded rather than fetched, so nothing is waiting on
// the network -- but decoding a dozen data: URIs and resolving the fonts still
// takes a moment, and printing before that finishes drops images off the page.
func waitPrintable() chromedp.Action {
	const script = `(async () => {
		await document.fonts.ready;
		await Promise.all([...document.images].map(img =>
			img.complete ? null : new Promise(done => {
				img.onload = done;
				img.onerror = done;
			})));
		return true;
	})()`

	var ready bool
	return chromedp.Evaluate(script, &ready, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})
}
