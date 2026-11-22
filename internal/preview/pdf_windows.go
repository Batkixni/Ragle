//go:build windows

package preview

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// winrtPDFScript renders a single PDF page to PNG using Windows 10/11's built-in
// Windows.Data.Pdf.PdfDocument WinRT API with zero external dependencies.
const winrtPDFScript = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
[void][Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
[void][Windows.Data.Pdf.PdfDocument,Windows.Data.Pdf,ContentType=WindowsRuntime]
[void][Windows.Data.Pdf.PdfPageRenderOptions,Windows.Data.Pdf,ContentType=WindowsRuntime]
[void][Windows.Storage.Streams.InMemoryRandomAccessStream,Windows.Storage.Streams,ContentType=WindowsRuntime]

$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'
})[0]
$asTaskAction = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction'
})[0]

function AwaitOp($op, $resultType) {
  $task = $asTaskGeneric.MakeGenericMethod($resultType).Invoke($null, @($op))
  $task.Wait()
  return $task.Result
}
function AwaitAct($act) {
  $task = $asTaskAction.Invoke($null, @($act))
  $task.Wait()
}

$pdfPath = $env:RAGLEX_PDF_PATH
$outPath = $env:RAGLEX_PDF_OUT
$pageIdx = [uint32]$env:RAGLEX_PDF_PAGE
$maxEdge = [double]$env:RAGLEX_PDF_MAX

$file = AwaitOp ([Windows.Storage.StorageFile]::GetFileFromPathAsync($pdfPath)) ([Windows.Storage.StorageFile])
$doc = AwaitOp ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($file)) ([Windows.Data.Pdf.PdfDocument])
$count = [uint32]$doc.PageCount
if ($count -eq 0) { throw "empty pdf" }
if ($pageIdx -ge $count) { $pageIdx = $count - 1 }

$page = $doc.GetPage($pageIdx)
try {
  $opts = [Windows.Data.Pdf.PdfPageRenderOptions]::new()
  $w = [double]$page.Size.Width
  $h = [double]$page.Size.Height
  if ($w -gt 0 -and $h -gt 0) {
    $scale = $maxEdge / [Math]::Max($w, $h)
    if ($scale -lt 0.5) { $scale = 0.5 }
    if ($scale -gt 3.0) { $scale = 3.0 }
    $opts.DestinationWidth = [uint32][Math]::Round($w * $scale)
    $opts.DestinationHeight = [uint32][Math]::Round($h * $scale)
  }
  $mem = [Windows.Storage.Streams.InMemoryRandomAccessStream]::new()
  try {
    AwaitAct ($page.RenderToStreamAsync($mem, $opts))
    $netStream = [System.IO.WindowsRuntimeStreamExtensions]::AsStreamForRead($mem)
    $netStream.Position = 0
    $fs = [System.IO.File]::Create($outPath)
    try {
      $netStream.CopyTo($fs)
    } finally {
      $fs.Dispose()
      $netStream.Dispose()
    }
  } finally {
    $mem.Dispose()
  }
} finally {
  $page.Dispose()
}
Write-Output ("PAGES=" + $count)
`

func renderPDFNative(path string, pageIndex int, maxEdge int) (*image.RGBA, int, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	tmpDir, err := os.MkdirTemp("", "raglex-pdf-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmpDir)

	outPNG := filepath.Join(tmpDir, "page.png")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", winrtPDFScript)
	hideCmd(cmd)
	cmd.Env = append(os.Environ(),
		"RAGLEX_PDF_PATH="+absPath,
		"RAGLEX_PDF_OUT="+outPNG,
		"RAGLEX_PDF_PAGE="+strconv.Itoa(pageIndex),
		"RAGLEX_PDF_MAX="+strconv.Itoa(maxEdge),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, 0, fmt.Errorf("windows pdf render: %s", msg)
		}
		return nil, 0, errPDFMsg(err)
	}

	total := CountPDFPagesFallback(absPath)
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PAGES=") {
			if n, perr := strconv.Atoi(strings.TrimPrefix(line, "PAGES=")); perr == nil && n > 0 {
				total = n
			}
		}
	}
	img, err := decodeRGBA(outPNG)
	if err != nil {
		return nil, total, err
	}
	return img, total, nil
}
