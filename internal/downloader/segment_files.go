package downloader

import (
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "unicode"
)

// sanitizeSegmentName keeps the folder and file component valid on Windows.
// It is deliberately stricter than POSIX rules to keep Portable builds safe.
func sanitizeSegmentName(raw string) string {
  const maxRunes = 90
  safe := strings.Map(func(r rune) rune {
    if r < 32 || r == 127 || strings.ContainsRune("<>:\"/\\|?*", r) {
      return '-'
    }
    if unicode.IsSpace(r) {
      return ' '
    }
    return r
  }, strings.TrimSpace(raw))
  safe = strings.Trim(safe, " .")
  chars := []rune(safe)
  if len(chars) > maxRunes {
    safe = strings.TrimRight(string(chars[:maxRunes]), " .")
  }
  if safe == "" { safe = "제목 없음" }
  prefix := strings.ToUpper(strings.SplitN(safe, ".", 2)[0])
  switch prefix {
  case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
    safe = "_" + safe
  }
  return safe
}

func (q *Queue) reserveSegmentFileName(dir string, segment DownloadSegment) string {
  base := sanitizeSegmentName(segment.Title)
  // Preserve the default for clients and drafts created before the checkbox
  // existed. Explicit false omits the position prefix.
  if segment.NumberFileName == nil || *segment.NumberFileName {
    base = fmt.Sprintf("%02d. %s", segment.Position, base)
  }
  // An os.ReadDir check avoids an existing completed file being overwritten.
  files, _ := os.ReadDir(dir)
  reserved := make(map[string]struct{})
  for _, task := range q.tasks {
    if task.Segment != nil && strings.EqualFold(filepath.Clean(task.OutputDir), filepath.Clean(dir)) {
      reserved[strings.ToLower(task.Segment.FileName)] = struct{}{}
    }
  }
  for n := 1; ; n++ {
    stem := base
    if n > 1 { stem = fmt.Sprintf("%s (%d)", base, n) }
    if _, found := reserved[strings.ToLower(stem)]; found { continue }
    inUse := false
    for _, f := range files {
      if strings.HasPrefix(strings.ToLower(f.Name()), strings.ToLower(stem)+".") {
        inUse = true
        break
      }
    }
    if !inUse { return stem }
  }
}
