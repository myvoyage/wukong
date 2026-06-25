// Package zim provides ZIM file format creation following the
// ZIM specification (https://wiki.openzim.org/wiki/ZIM_file_format).
//
// This implementation creates ZIM archives compatible with Kiwix
// and other ZIM readers. Clusters are compressed with zstd, and a
// full-file MD5 checksum is appended for data integrity.
//
// Basic usage:
//
//	packer := zim.NewPacker()
//	packer.AddArticle("index.html", "Home", "text/html", htmlData)
//	packer.Build("output.zim", "MyApp", "Description", true)
//
// For reading:
//
//	r, _ := zim.Open("output.zim")
//	defer r.Close()
//	blob, _ := r.Get('A', "index.html")
package zim

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Packer (write path)
// ---------------------------------------------------------------------------

// Packer creates ZIM archive files. It manages articles, MIME types,
// and produces a complete, well-formed ZIM file.
type Packer struct {
	articles  []article
	byKey     map[string]int // key → index in articles slice
	mimeTypes map[string]uint16

	// mainPageNS and mainPageURL are set via SetMainPage.
	// When empty, findMainPage auto-detects the main page.
	mainPageNS  byte
	mainPageURL string
}

// NewPacker creates a new ZIM packer ready to accept articles.
func NewPacker() *Packer {
	return &Packer{
		articles:  make([]article, 0),
		byKey:     make(map[string]int),
		mimeTypes: make(map[string]uint16),
	}
}

// SetMainPage marks an existing entry as the archive's main page.
// The namespace and URL must match a previously added article.
func (p *Packer) SetMainPage(namespace byte, url string) {
	p.mainPageNS = namespace
	p.mainPageURL = url
}

// AddContent adds a content article with explicit namespace support.
// If an article with the same namespace+URL already exists, it is
// replaced in place. Title defaults to URL if empty. MIME defaults
// to "text/html" if empty.
func (p *Packer) AddContent(namespace byte, url, title, mimeType string,
	data []byte) error {
	if url == "" {
		return errors.New("zim: article URL cannot be empty")
	}
	if title == "" {
		title = url
	}
	if mimeType == "" {
		mimeType = "text/html"
	}

	p.registerMimeType(mimeType)

	e := article{
		Title:       title,
		URL:         url,
		Namespace:   namespace,
		ArticleType: ArticleTypeArticle,
		MimeType:    p.mimeTypes[mimeType],
		Data:        data,
	}
	p.put(e)
	return nil
}

// AddMetadata adds a metadata entry in the M namespace.
// value is stored as text/plain.
func (p *Packer) AddMetadata(name, value string) {
	e := article{
		Title:       name,
		URL:         name,
		Namespace:   'M',
		ArticleType: ArticleTypeArticle,
		MimeType:    p.mimeTypes["text/plain"],
		Data:        []byte(value),
	}
	p.registerMimeType("text/plain")
	e.MimeType = p.mimeTypes["text/plain"]
	p.put(e)
}

// AddArticle adds a content article to the ZIM archive in namespace 'C'.
// Deprecated: prefer AddContent('C', url, title, mimeType, data).
func (p *Packer) AddArticle(url, title, mimeType string, data []byte) error {
	return p.AddContent('C', url, title, mimeType, data)
}

// AddRedirect adds a redirect article that points to another article
// by namespace and URL. The target entry will be resolved at build time.
func (p *Packer) AddRedirect(ns byte, url, title string,
	targetNS byte, targetURL string) error {
	if url == "" {
		return errors.New("zim: redirect URL cannot be empty")
	}
	if title == "" {
		title = url
	}

	e := article{
		Title:       title,
		URL:         url,
		Namespace:   ns,
		ArticleType: ArticleTypeRedirect,
		// Store target as string key for later resolution.
		Redirect: 0,
	}
	// Hijack the Data field to store the target key.
	e.Data = []byte(key(targetNS, targetURL))
	p.put(e)
	return nil
}

// ArticleCount returns the number of articles currently added.
func (p *Packer) ArticleCount() int {
	return len(p.articles)
}

// put inserts or replaces an entry keyed by namespace+URL.
func (p *Packer) put(e article) {
	k := key(e.Namespace, e.URL)
	if idx, ok := p.byKey[k]; ok {
		p.articles[idx] = e
		return
	}
	p.byKey[k] = len(p.articles)
	p.articles = append(p.articles, e)
}

// Build creates the ZIM archive and writes it to the output file.
func (p *Packer) Build(outputPath, appName, appDescription string,
	compress bool) error {
	_, err := p.BuildWithStats(outputPath, BuildOptions{
		AppName:        appName,
		AppDescription: appDescription,
		Compress:       compress,
	}, "", false)
	return err
}

// BuildOptions holds options for ZIM archive construction.
type BuildOptions struct {
	AppName        string
	AppDescription string
	Compress       bool
}

// PackStats reports incremental packing statistics.
type PackStats struct {
	ClustersReused     int
	ClustersCompressed int
}

// BuildWithStats creates the ZIM archive with incremental caching and
// returns packing statistics. If cachePath is non-empty and incremental
// is true, unchanged clusters are reused from the cache.
func (p *Packer) BuildWithStats(outputPath string, opts BuildOptions,
	cachePath string, incremental bool) (PackStats, error) {
	var stats PackStats

	file, err := os.Create(outputPath)
	if err != nil {
		return stats, fmt.Errorf("zim: create output file: %w", err)
	}
	defer file.Close()

	if _, err := p.writeToWithCache(file, opts.Compress, cachePath,
		incremental, &stats); err != nil {
		return stats, err
	}
	return stats, nil
}

// WriteTo serialises the entire ZIM archive to w.
func (p *Packer) WriteTo(w io.Writer, compress bool) (int64, error) {
	var stats PackStats
	return p.writeToWithCache(w, compress, "", false, &stats)
}

// writeToWithCache is the internal serialisation entry with cluster caching.
func (p *Packer) writeToWithCache(w io.Writer, compress bool,
	cachePath string, incremental bool, stats *PackStats) (int64, error) {
	if len(p.articles) == 0 {
		return 0, errors.New("zim: no articles to pack")
	}

	// Load cluster cache if incremental is enabled.
	var cache *clusterCache
	if incremental && cachePath != "" {
		cache = loadClusterCache(cachePath)
	}

	// Sort articles by namespace+URL (ZIM spec requirement).
	sort.Slice(p.articles, func(i, j int) bool {
		a, b := p.articles[i], p.articles[j]
		return key(a.Namespace, a.URL) < key(b.Namespace, b.URL)
	})

	if err := p.resolveRedirects(); err != nil {
		return 0, err
	}

	mimeList := p.buildMimeList()
	clusters, reused := p.buildClustersWithCache(compress, cache, stats)

	// Persist cache for next build (keys are uncompressed hashes).
	if incremental && cachePath != "" {
		saveClusterCache(cachePath, cache)
	}

	if stats != nil {
		stats.ClustersReused = reused
		stats.ClustersCompressed += len(clusters) - reused
	}

	// Calculate file layout using precomputed offsets (O(N)).
	// ZIM v6 layout: Header → MIME List → URL Ptr → Title Ptr → Cluster Ptr → Articles → Clusters → MD5
	// All pointer lists must be 8-byte aligned.
	articleOffsets, articleDataSize := p.calculateArticleLayout()
	urlPtrListSize := uint64(len(p.articles) * 8)
	titlePtrListSize := uint64(len(p.articles) * 8)
	clusterPtrListSize := uint64(len(clusters) * 8)
	mimeListSize := uint64(len(mimeList))

	mimeListPos := uint64(HeaderSize)
	urlPtrPos := align8(mimeListPos + mimeListSize)
	titlePtrPos := urlPtrPos + urlPtrListSize
	clusterPtrPos := titlePtrPos + titlePtrListSize
	articlesPos := clusterPtrPos + clusterPtrListSize
	clusterDataPos := articlesPos + articleDataSize

	// Also write MIME list with 8-byte alignment padding.
	paddedMime := make([]byte, urlPtrPos-mimeListPos)
	copy(paddedMime, mimeList)

	checksumPos := clusterDataPos
	for _, cl := range clusters {
		checksumPos += uint64(len(cl.data))
	}

	mainPage := p.resolveMainPage()

	var buf bytes.Buffer

	header := Header{
		Magic:         [4]byte{0x5a, 0x49, 0x4d, 0x04},
		MajorVersion:  6,
		MinorVersion:  0,
		UUID:          p.computeUUID(),
		ArticleCount:  uint32(len(p.articles)),
		ClusterCount:  uint32(len(clusters)),
		URLPtrPos:     urlPtrPos,
		TitlePtrPos:   titlePtrPos,
		ClusterPtrPos: clusterPtrPos,
		MimeListPos:   mimeListPos,
		MainPage:      mainPage,
		LayoutPage:    noMainPage,
		ChecksumPos:   checksumPos,
	}
	if err := writeHeader(&buf, &header); err != nil {
		return 0, fmt.Errorf("zim: write header: %w", err)
	}

	// Write MIME type list (with 8-byte alignment padding).
	if _, err := buf.Write(paddedMime); err != nil {
		return 0, fmt.Errorf("zim: write MIME list: %w", err)
	}

	// Write URL pointer list (O(N) using precomputed offsets).
	for i := range p.articles {
		pos := articlesPos + calculateArticleOffset(articleOffsets, i)
		if err := binary.Write(&buf, binary.LittleEndian, pos); err != nil {
			return 0, fmt.Errorf("zim: write URL pointer: %w", err)
		}
	}

	// Write title pointer list (sorted by namespace+title).
	titleOrder := make([]int, len(p.articles))
	for i := range titleOrder {
		titleOrder[i] = i
	}
	sort.Slice(titleOrder, func(i, j int) bool {
		a, b := p.articles[titleOrder[i]], p.articles[titleOrder[j]]
		ka := key(a.Namespace, a.Title)
		kb := key(b.Namespace, b.Title)
		if ka != kb {
			return ka < kb
		}
		return titleOrder[i] < titleOrder[j]
	})
	for _, idx := range titleOrder {
		pos := articlesPos + calculateArticleOffset(articleOffsets, idx)
		if err := binary.Write(&buf, binary.LittleEndian, pos); err != nil {
			return 0, fmt.Errorf("zim: write title pointer: %w", err)
		}
	}

	// Write cluster pointers.
	currentOffset := articlesPos + articleDataSize
	for i := range clusters {
		if err := binary.Write(&buf, binary.LittleEndian,
			currentOffset); err != nil {
			return 0, fmt.Errorf("zim: write cluster pointer: %w", err)
		}
		currentOffset += uint64(len(clusters[i].data))
	}

	// Write articles.
	for i := range p.articles {
		if err := writeArticle(&buf, &p.articles[i]); err != nil {
			return 0, fmt.Errorf("zim: write article %q: %w",
				p.articles[i].URL, err)
		}
	}

	// Write cluster data.
	for i := range clusters {
		if _, err := buf.Write(clusters[i].data); err != nil {
			return 0, fmt.Errorf("zim: write cluster %d: %w", i, err)
		}
	}

	// Compute MD5 checksum.
	checksum := md5.Sum(buf.Bytes())
	bodyLen := int64(buf.Len())

	if _, err := w.Write(buf.Bytes()); err != nil {
		return 0, fmt.Errorf("zim: write file body: %w", err)
	}
	if _, err := w.Write(checksum[:]); err != nil {
		return 0, fmt.Errorf("zim: write checksum: %w", err)
	}

	return bodyLen, nil
}

// resolveMainPage returns the article index of the main page.
// Uses byKey map for O(1) lookup after SetMainPage.
func (p *Packer) resolveMainPage() uint32 {
	if p.mainPageURL != "" {
		k := key(p.mainPageNS, p.mainPageURL)
		if idx, ok := p.byKey[k]; ok {
			return uint32(idx)
		}
	}
	return findMainPage(p.articles)
}

// resolveRedirects resolves redirect target keys to article indices
// after sorting. Uses the byKey map for O(1) target lookup.
func (p *Packer) resolveRedirects() error {
	for i := range p.articles {
		a := &p.articles[i]
		if a.ArticleType != ArticleTypeRedirect || len(a.Data) == 0 {
			continue
		}
		targetKey := string(a.Data)
		targetIdx, ok := p.byKey[targetKey]
		if !ok {
			return fmt.Errorf(
				"zim: redirect target %q not found", targetKey)
		}
		a.Redirect = uint32(targetIdx)
		a.Data = nil
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal writer helpers
// ---------------------------------------------------------------------------

func (p *Packer) registerMimeType(mime string) {
	if _, ok := p.mimeTypes[mime]; !ok {
		p.mimeTypes[mime] = uint16(len(p.mimeTypes))
	}
}

func (p *Packer) buildMimeList() []byte {
	var buf bytes.Buffer

	// Empty first entry (index 0 is always empty per ZIM spec).
	buf.WriteByte(0)

	// MIME types are output in registration order so article MIME
	// indices (assigned during AddContent/AddMetadata) match correctly.
	// Build a reverse index to emit in index order [0..N-1].
	n := len(p.mimeTypes)
	mimeByIndex := make([]string, n)
	for mime, idx := range p.mimeTypes {
		mimeByIndex[idx] = mime
	}
	for i := 0; i < n; i++ {
		if mimeByIndex[i] != "" {
			buf.WriteString(mimeByIndex[i])
			buf.WriteByte(0)
		}
	}
	return buf.Bytes()
}

// buildClustersWithCache builds clusters with optional incremental caching.
// Hash-and-cache strategy:
//  1. Compute SHA-256 of each uncompressed cluster BEFORE compression.
//  2. Look up the uncompressed hash in the cache.
//  3. On hit → reuse the cached (already-compressed) bytes, skip zstd.
//  4. On miss → compress with zstd → store in cache keyed by UNCOMPRESSED hash.
//
// This guarantees cache hits on identical content across builds, and avoids
// redundant zstd compression overhead for unchanged clusters (potentially
// saving seconds per cluster for large archives).
func (p *Packer) buildClustersWithCache(compress bool, cache *clusterCache,
	_ *PackStats) ([]cluster, int) {
	const maxClusterSize = 2 * 1024 * 1024 // 2 MiB per ZIM cluster

	// Build reverse map from MIME index to MIME string for
	// classifying content as text or binary.
	mimeByIndex := make(map[uint16]string, len(p.mimeTypes))
	for mime, idx := range p.mimeTypes {
		mimeByIndex[idx] = mime
	}

	// Separate buffers: text content goes to compressed clusters,
	// binary content (images, fonts, etc.) goes to uncompressed clusters.
	var textBuf, binaryBuf []byte
	var textClusters, binaryClusters []cluster

	for i := range p.articles {
		if p.articles[i].ArticleType != ArticleTypeArticle ||
			len(p.articles[i].Data) == 0 {
			continue
		}

		data := p.articles[i].Data
		mime := mimeByIndex[p.articles[i].MimeType]

		if isTextMime(mime) {
			// Text content — goes to a compressible cluster.
			if len(textBuf)+len(data)+4 > maxClusterSize {
				if len(textBuf) > 0 {
					textClusters = append(textClusters,
						cluster{data: textBuf})
				}
				textBuf = nil
			}
			blobHeader := make([]byte, 4)
			binary.LittleEndian.PutUint32(blobHeader, uint32(len(data)))
			textBuf = append(textBuf, blobHeader...)
			textBuf = append(textBuf, data...)
		} else {
			// Binary content — stored uncompressed.
			if len(binaryBuf)+len(data)+4 > maxClusterSize {
				if len(binaryBuf) > 0 {
					binaryClusters = append(binaryClusters,
						cluster{data: binaryBuf})
				}
				binaryBuf = nil
			}
			blobHeader := make([]byte, 4)
			binary.LittleEndian.PutUint32(blobHeader, uint32(len(data)))
			binaryBuf = append(binaryBuf, blobHeader...)
			binaryBuf = append(binaryBuf, data...)
		}
	}

	// Flush remaining buffers.
	if len(textBuf) > 0 {
		textClusters = append(textClusters, cluster{data: textBuf})
	}
	if len(binaryBuf) > 0 {
		binaryClusters = append(binaryClusters, cluster{data: binaryBuf})
	}

	// Merge text and binary clusters in order (text first).
	allClusters := append(textClusters, binaryClusters...)

	// Pre-compute uncompressed hashes for text clusters (needed for
	// cache lookup BEFORE compression).
	uncompHashes := make([]string, len(textClusters))
	for i := range textClusters {
		uncompHashes[i] = clusterHash(textClusters[i].data)
	}

	// Check cache using uncompressed hashes BEFORE compression.
	// This avoids redundant zstd work for unchanged clusters.
	reused := 0
	reusedIdx := make(map[int]bool)
	if cache != nil {
		for i := range textClusters {
			hash := uncompHashes[i]
			if cached, ok := cache.entries[hash]; ok {
				allClusters[i].data = cached
				// Re-add to cache so saveClusterCache includes it.
				cache.entries[hash] = cached
				reused++
				reusedIdx[i] = true
			}
		}
	}

	// Use the shared zstd encoder for text clusters (skip reused ones).
	enc := getZstdEncoder()
	for i := range textClusters {
		if reusedIdx[i] {
			continue
		}
		if compress && enc != nil {
			allClusters[i].data = enc.EncodeAll(allClusters[i].data, nil)
			// Store in cache keyed by UNCOMPRESSED hash, so the same
			// content is found on the next build regardless of compression
			// output (zstd output is deterministic for the same input).
			if cache != nil {
				cache.entries[uncompHashes[i]] = copyBytes(allClusters[i].data)
			}
		}
		compByte := byte(CompressionZstd)
		if !compress || enc == nil {
			compByte = byte(CompressionNone)
		}
		allClusters[i].data = append([]byte{compByte}, allClusters[i].data...)
	}

	// Binary clusters: always uncompressed.
	binaryIdx := len(textClusters)
	for i := range binaryClusters {
		if reusedIdx[binaryIdx+i] {
			continue
		}
		allClusters[binaryIdx+i].data = append(
			[]byte{byte(CompressionNone)}, allClusters[binaryIdx+i].data...)
	}

	return allClusters, reused
}

// ---------------------------------------------------------------------------
// Cluster cache for incremental ZIM builds.
// ---------------------------------------------------------------------------

type clusterCache struct {
	entries map[string][]byte // SHA-256 hash → compressed cluster bytes.
}

func loadClusterCache(path string) *clusterCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return &clusterCache{entries: make(map[string][]byte)}
	}
	cc := &clusterCache{entries: make(map[string][]byte)}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		hash, b64 := line[:idx], line[idx+1:]
		decoded := decodeB64(b64)
		if len(decoded) > 0 {
			cc.entries[hash] = decoded
		}
	}
	return cc
}

// saveClusterCache persists the cache entries to disk.
// Each line is "sha256hash:base64(compressed_bytes)".
// Keys are uncompressed cluster hashes for consistent lookup across builds.
func saveClusterCache(path string, cache *clusterCache) {
	if cache == nil || len(cache.entries) == 0 {
		return
	}
	var lines []string
	for hash, data := range cache.entries {
		lines = append(lines, hash+":"+encodeB64(data))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func clusterHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func encodeB64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func decodeB64(s string) []byte {
	d, _ := base64.StdEncoding.DecodeString(s)
	return d
}

func copyBytes(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

func align8(v uint64) uint64 {
	return (v + 7) & ^uint64(7)
}

// calculateArticleLayout precomputes article sizes and offsets for
// the entire article set. Returns offset to each article (by index),
// and total size of all articles. Uses O(N) prefix sum.
func (p *Packer) calculateArticleLayout() (offsets []uint64, totalSize uint64) {
	offsets = make([]uint64, len(p.articles))
	for i := range p.articles {
		offsets[i] = totalSize
		totalSize += p.calculateSingleArticleSize(i)
	}
	return offsets, totalSize
}

func (p *Packer) calculateSingleArticleSize(idx int) uint64 {
	a := &p.articles[idx]
	// Fixed prefix: 16B header + 8B extData + url + title.
	prefix := uint64(articleHeaderSize + 8 + len(a.URL) + len(a.Title))
	// Align prefix to 4 bytes (pad after url+title).
	alignedPrefix := (prefix + 3) & ^uint64(3)
	if a.ArticleType == ArticleTypeRedirect || len(a.Data) == 0 {
		return alignedPrefix
	}
	// Add data length (inline, no final padding needed).
	return alignedPrefix + uint64(len(a.Data))
}

// calculateArticleOffset returns the byte offset of the article at idx.
// Uses the precomputed offsets slice for O(1) lookup.
func calculateArticleOffset(offsets []uint64, idx int) uint64 {
	return offsets[idx]
}

// writeArticle serialises a single directory entry to w.
// Layout: [16B header][8B extData][url][title][pad:4B][data][pad:4B]
// Redirect entries skip the data section.
func writeArticle(w *bytes.Buffer, a *article) error {
	header := make([]byte, articleHeaderSize)
	binary.LittleEndian.PutUint16(header[0:2], uint16(len(a.Title)))
	binary.LittleEndian.PutUint16(header[2:4], uint16(len(a.URL)))
	header[4] = a.Namespace
	binary.LittleEndian.PutUint32(header[5:9], 0) // revision
	header[9] = byte(a.ArticleType)
	binary.LittleEndian.PutUint16(header[10:12], a.MimeType)
	binary.LittleEndian.PutUint32(header[12:16], a.Redirect)
	w.Write(header)

	// Extended data (8 bytes).
	extData := make([]byte, 8)
	if a.ArticleType == ArticleTypeRedirect {
		binary.LittleEndian.PutUint32(extData[0:4], a.Redirect)
	}
	w.Write(extData)

	// URL and title strings.
	w.WriteString(a.URL)
	w.WriteString(a.Title)

	// Pad strings section to 4-byte alignment.
	if pad := (4 - (w.Len() % 4)) % 4; pad > 0 {
		w.Write(make([]byte, pad))
	}

	// Article data (inline, only for non-redirect content articles).
	if a.ArticleType != ArticleTypeRedirect && len(a.Data) > 0 {
		w.Write(a.Data)
	}
	// Note: no final padding needed since inline data dirents
	// use the URL pointer list for boundary determination.

	return nil
}

// ---------------------------------------------------------------------------
// UUID generation
// ---------------------------------------------------------------------------

// computeUUID derives a deterministic UUID from all article content.
// Identical input produces identical output, making ZIM builds reproducible.
func (p *Packer) computeUUID() [16]byte {
	h := md5.New()
	for i := range p.articles {
		a := &p.articles[i]
		h.Write([]byte(a.URL))
		h.Write([]byte(a.Title))
		var lenBuf [8]byte
		binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(a.Data)))
		h.Write(lenBuf[:])
		h.Write(a.Data)
	}
	var uuid [16]byte
	copy(uuid[:], h.Sum(nil))
	return uuid
}
