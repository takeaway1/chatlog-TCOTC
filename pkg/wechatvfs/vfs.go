// Package wechatvfs 提供微信加密数据库的 SQLite VFS 实现
// 允许直接读取加密的微信数据库文件，无需先解密到磁盘
package wechatvfs

/*
#cgo CFLAGS: -DSQLITE_ENABLE_UNLOCK_NOTIFY
#include <sqlite3.h>
#include <stdlib.h>
#include <string.h>

// Go 回调函数声明 - 使用 GoInt 和 GoInt64 类型
extern int goWechatFileRead(int fileId, void* buf, int amt, long long offset);
extern int goWechatFileClose(int fileId);
extern long long goWechatFileSize(int fileId);
extern int goWechatFileOpen(char* name, int flags);

// 文件方法前向声明
static int wechatFileClose(sqlite3_file*);
static int wechatFileRead(sqlite3_file*, void*, int iAmt, sqlite3_int64 iOfst);
static int wechatFileWrite(sqlite3_file*, const void*, int iAmt, sqlite3_int64 iOfst);
static int wechatFileTruncate(sqlite3_file*, sqlite3_int64 size);
static int wechatFileSync(sqlite3_file*, int flags);
static int wechatFileSize(sqlite3_file*, sqlite3_int64 *pSize);
static int wechatFileLock(sqlite3_file*, int);
static int wechatFileUnlock(sqlite3_file*, int);
static int wechatFileCheckReservedLock(sqlite3_file*, int *pResOut);
static int wechatFileControl(sqlite3_file*, int op, void *pArg);
static int wechatFileSectorSize(sqlite3_file*);
static int wechatFileDeviceCharacteristics(sqlite3_file*);

// 文件 IO 方法表
static const sqlite3_io_methods wechatIoMethods = {
    1,                              // iVersion
    wechatFileClose,
    wechatFileRead,
    wechatFileWrite,
    wechatFileTruncate,
    wechatFileSync,
    wechatFileSize,
    wechatFileLock,
    wechatFileUnlock,
    wechatFileCheckReservedLock,
    wechatFileControl,
    wechatFileSectorSize,
    wechatFileDeviceCharacteristics,
    // v2 和 v3 方法留空
    0, 0, 0, 0, 0, 0
};

// 自定义文件结构
typedef struct wechatFile {
    sqlite3_file base;      // 必须是第一个成员
    int fileId;             // Go 端文件句柄 ID
} wechatFile;

// 文件方法实现
static int wechatFileClose(sqlite3_file* pFile) {
    wechatFile* p = (wechatFile*)pFile;
    return goWechatFileClose(p->fileId);
}

static int wechatFileRead(sqlite3_file* pFile, void* buf, int iAmt, sqlite3_int64 iOfst) {
    wechatFile* p = (wechatFile*)pFile;
    return goWechatFileRead(p->fileId, buf, iAmt, (long long)iOfst);
}

static int wechatFileWrite(sqlite3_file* pFile, const void* buf, int iAmt, sqlite3_int64 iOfst) {
    // 只读模式，不支持写入
    return SQLITE_READONLY;
}

static int wechatFileTruncate(sqlite3_file* pFile, sqlite3_int64 size) {
    return SQLITE_READONLY;
}

static int wechatFileSync(sqlite3_file* pFile, int flags) {
    return SQLITE_OK;
}

static int wechatFileSize(sqlite3_file* pFile, sqlite3_int64* pSize) {
    wechatFile* p = (wechatFile*)pFile;
    *pSize = (sqlite3_int64)goWechatFileSize(p->fileId);
    return SQLITE_OK;
}

static int wechatFileLock(sqlite3_file* pFile, int level) {
    return SQLITE_OK;
}

static int wechatFileUnlock(sqlite3_file* pFile, int level) {
    return SQLITE_OK;
}

static int wechatFileCheckReservedLock(sqlite3_file* pFile, int* pResOut) {
    *pResOut = 0;
    return SQLITE_OK;
}

static int wechatFileControl(sqlite3_file* pFile, int op, void* pArg) {
    return SQLITE_NOTFOUND;
}

static int wechatFileSectorSize(sqlite3_file* pFile) {
    return 4096;
}

static int wechatFileDeviceCharacteristics(sqlite3_file* pFile) {
    return SQLITE_IOCAP_IMMUTABLE;
}

// VFS 方法前向声明
static int wechatVfsOpen(sqlite3_vfs*, const char *zName, sqlite3_file*, int flags, int *pOutFlags);
static int wechatVfsDelete(sqlite3_vfs*, const char *zName, int syncDir);
static int wechatVfsAccess(sqlite3_vfs*, const char *zName, int flags, int *pResOut);
static int wechatVfsFullPathname(sqlite3_vfs*, const char *zName, int nOut, char *zOut);
static void* wechatVfsDlOpen(sqlite3_vfs*, const char *zFilename);
static void wechatVfsDlError(sqlite3_vfs*, int nByte, char *zErrMsg);
static void (*wechatVfsDlSym(sqlite3_vfs*, void*, const char *zSymbol))(void);
static void wechatVfsDlClose(sqlite3_vfs*, void*);
static int wechatVfsRandomness(sqlite3_vfs*, int nByte, char *zOut);
static int wechatVfsSleep(sqlite3_vfs*, int microseconds);
static int wechatVfsCurrentTime(sqlite3_vfs*, double*);
static int wechatVfsGetLastError(sqlite3_vfs*, int, char*);
static int wechatVfsCurrentTimeInt64(sqlite3_vfs*, sqlite3_int64*);

// VFS 方法实现
static int wechatVfsOpen(sqlite3_vfs* pVfs, const char* zName, sqlite3_file* pFile, int flags, int* pOutFlags) {
    wechatFile* p = (wechatFile*)pFile;
    memset(p, 0, sizeof(*p));
    p->base.pMethods = &wechatIoMethods;

    int fileId = goWechatFileOpen((char*)zName, flags);
    if (fileId < 0) {
        return SQLITE_CANTOPEN;
    }
    p->fileId = fileId;

    if (pOutFlags) {
        *pOutFlags = SQLITE_OPEN_READONLY;
    }
    return SQLITE_OK;
}

static int wechatVfsDelete(sqlite3_vfs* pVfs, const char* zName, int syncDir) {
    return SQLITE_READONLY;
}

static int wechatVfsAccess(sqlite3_vfs* pVfs, const char* zName, int flags, int* pResOut) {
    *pResOut = 0;
    if (flags == SQLITE_ACCESS_EXISTS || flags == SQLITE_ACCESS_READ) {
        // 简化：总是返回存在且可读
        *pResOut = 1;
    }
    return SQLITE_OK;
}

static int wechatVfsFullPathname(sqlite3_vfs* pVfs, const char* zName, int nOut, char* zOut) {
    if (zName == 0) {
        return SQLITE_CANTOPEN;
    }
    int n = strlen(zName);
    if (n >= nOut) {
        return SQLITE_CANTOPEN;
    }
    strcpy(zOut, zName);
    return SQLITE_OK;
}

static void* wechatVfsDlOpen(sqlite3_vfs* pVfs, const char* zFilename) {
    return 0;
}

static void wechatVfsDlError(sqlite3_vfs* pVfs, int nByte, char* zErrMsg) {
    if (nByte > 0) zErrMsg[0] = 0;
}

static void (*wechatVfsDlSym(sqlite3_vfs* pVfs, void* p, const char* zSymbol))(void) {
    return 0;
}

static void wechatVfsDlClose(sqlite3_vfs* pVfs, void* p) {
}

static int wechatVfsRandomness(sqlite3_vfs* pVfs, int nByte, char* zOut) {
    sqlite3_vfs* pDefault = sqlite3_vfs_find(0);
    if (pDefault) {
        return pDefault->xRandomness(pDefault, nByte, zOut);
    }
    return SQLITE_OK;
}

static int wechatVfsSleep(sqlite3_vfs* pVfs, int microseconds) {
    sqlite3_vfs* pDefault = sqlite3_vfs_find(0);
    if (pDefault) {
        return pDefault->xSleep(pDefault, microseconds);
    }
    return microseconds;
}

static int wechatVfsCurrentTime(sqlite3_vfs* pVfs, double* pTime) {
    sqlite3_vfs* pDefault = sqlite3_vfs_find(0);
    if (pDefault) {
        return pDefault->xCurrentTime(pDefault, pTime);
    }
    return SQLITE_ERROR;
}

static int wechatVfsGetLastError(sqlite3_vfs* pVfs, int nByte, char* zErrMsg) {
    if (nByte > 0) zErrMsg[0] = 0;
    return 0;
}

static int wechatVfsCurrentTimeInt64(sqlite3_vfs* pVfs, sqlite3_int64* pTime) {
    sqlite3_vfs* pDefault = sqlite3_vfs_find(0);
    if (pDefault && pDefault->xCurrentTimeInt64) {
        return pDefault->xCurrentTimeInt64(pDefault, pTime);
    }
    return SQLITE_ERROR;
}

// VFS 实例
static sqlite3_vfs wechatVfs = {
    2,                      // iVersion
    sizeof(wechatFile),     // szOsFile
    1024,                   // mxPathname
    0,                      // pNext
    "wechat",               // zName
    0,                      // pAppData
    wechatVfsOpen,
    wechatVfsDelete,
    wechatVfsAccess,
    wechatVfsFullPathname,
    wechatVfsDlOpen,
    wechatVfsDlError,
    wechatVfsDlSym,
    wechatVfsDlClose,
    wechatVfsRandomness,
    wechatVfsSleep,
    wechatVfsCurrentTime,
    wechatVfsGetLastError,
    wechatVfsCurrentTimeInt64,
    // v3 方法
    0, 0, 0
};

// 注册 VFS
static int registerWechatVfs() {
    return sqlite3_vfs_register(&wechatVfs, 0);
}

// 获取 VFS 名称
static const char* getWechatVfsName() {
    return wechatVfs.zName;
}
*/
import "C"
import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/pbkdf2"
)

// Debug 开关
var Debug = false

const (
	KeySize      = 32
	SaltSize     = 16
	AESBlockSize = 16
	IVSize       = 16

	SQLiteHeader = "SQLite format 3\x00"
)

// 平台版本参数
type DecryptParams struct {
	PageSize  int
	IterCount int           // PBKDF2 迭代次数，0 表示不使用 PBKDF2
	HmacSize  int
	HashFunc  func() hash.Hash
	Reserve   int
}

// 获取解密参数
func getDecryptParams(platform string, version int) *DecryptParams {
	var params DecryptParams

	switch {
	case platform == "windows" && version == 4:
		// Windows V4: SHA-512, 256000 iterations, page 4096
		params.PageSize = 4096
		params.IterCount = 256000
		params.HmacSize = 64 // SHA-512
		params.HashFunc = sha512.New

	case platform == "darwin" && version == 4:
		// Darwin V4: same as Windows V4
		params.PageSize = 4096
		params.IterCount = 256000
		params.HmacSize = 64 // SHA-512
		params.HashFunc = sha512.New

	case platform == "windows" && version == 3:
		// Windows V3: SHA-1, 64000 iterations, page 4096
		params.PageSize = 4096
		params.IterCount = 64000
		params.HmacSize = 20 // SHA-1
		params.HashFunc = sha1.New

	case platform == "darwin" && version == 3:
		// Darwin V3: SHA-1, NO PBKDF2 for encKey, page 1024
		params.PageSize = 1024
		params.IterCount = 0 // No PBKDF2 for encryption key
		params.HmacSize = 20 // SHA-1
		params.HashFunc = sha1.New

	default:
		// Default to Windows V4
		params.PageSize = 4096
		params.IterCount = 256000
		params.HmacSize = 64
		params.HashFunc = sha512.New
	}

	// Calculate reserve size
	params.Reserve = IVSize + params.HmacSize
	if params.Reserve%AESBlockSize != 0 {
		params.Reserve = ((params.Reserve / AESBlockSize) + 1) * AESBlockSize
	}

	return &params
}

// WechatFile 表示一个打开的加密微信数据库文件
type WechatFile struct {
	file       *os.File
	filePath   string
	fileSize   int64
	salt       []byte
	encKey     []byte
	macKey     []byte
	hashFunc   func() hash.Hash
	hmacSize   int
	reserve    int
	pageSize   int
	totalPages int64

	// 平台版本信息
	platform string
	version  int

	// 页面缓存
	cacheMu      sync.RWMutex
	cache        map[int64][]byte // pageNum -> decrypted page data
	cacheOrder   []int64          // LRU order
	maxCacheSize int
}

var (
	filesMu    sync.RWMutex
	files      = make(map[int]*WechatFile)
	nextFileID = 1

	// 全局配置：路径 -> 密钥和平台版本的映射
	keyConfigMu sync.RWMutex
	keyConfig   = make(map[string]*KeyConfig) // path -> config
)

// KeyConfig 存储密钥和平台版本信息
type KeyConfig struct {
	HexKey   string
	Platform string
	Version  int
}

// RegisterKey 注册数据库文件的解密密钥（默认 windows v4）
func RegisterKey(dbPath string, hexKey string) {
	RegisterKeyWithParams(dbPath, hexKey, "windows", 4)
}

// RegisterKeyWithParams 注册数据库文件的解密密钥和平台版本
func RegisterKeyWithParams(dbPath string, hexKey string, platform string, version int) {
	keyConfigMu.Lock()
	defer keyConfigMu.Unlock()
	keyConfig[dbPath] = &KeyConfig{
		HexKey:   hexKey,
		Platform: platform,
		Version:  version,
	}
}

// UnregisterKey 取消注册数据库文件的解密密钥
func UnregisterKey(dbPath string) {
	keyConfigMu.Lock()
	defer keyConfigMu.Unlock()
	delete(keyConfig, dbPath)
}

// getKeyConfig 获取数据库文件的配置
func getKeyConfig(dbPath string) (*KeyConfig, bool) {
	keyConfigMu.RLock()
	defer keyConfigMu.RUnlock()
	cfg, ok := keyConfig[dbPath]
	return cfg, ok
}

// RegisterVFS 注册微信 VFS 到 SQLite
func RegisterVFS() error {
	rc := C.registerWechatVfs()
	if rc != C.SQLITE_OK {
		return fmt.Errorf("failed to register wechat VFS: %d", rc)
	}
	return nil
}

// VFSName 返回 VFS 名称，用于 sql.Open
func VFSName() string {
	return C.GoString(C.getWechatVfsName())
}

// deriveKeys 派生加密密钥和 MAC 密钥
func deriveKeys(key []byte, salt []byte, params *DecryptParams) ([]byte, []byte) {
	var encKey []byte

	if params.IterCount > 0 {
		// 使用 PBKDF2 派生加密密钥
		encKey = pbkdf2.Key(key, salt, params.IterCount, KeySize, params.HashFunc)
	} else {
		// Darwin V3: 直接使用密钥
		encKey = make([]byte, KeySize)
		copy(encKey, key)
	}

	// 生成 MAC 密钥
	macSalt := xorBytes(salt, 0x3a)
	macKey := pbkdf2.Key(encKey, macSalt, 2, KeySize, params.HashFunc)

	return encKey, macKey
}

func xorBytes(a []byte, b byte) []byte {
	result := make([]byte, len(a))
	for i := range a {
		result[i] = a[i] ^ b
	}
	return result
}

// openWechatFile 打开并初始化加密数据库文件
func openWechatFile(path string) (*WechatFile, error) {
	if Debug {
		log.Debug().Str("path", path).Msg("vfs: openWechatFile")
	}

	cfg, ok := getKeyConfig(path)
	if !ok {
		return nil, fmt.Errorf("no key registered for %s", path)
	}

	if Debug {
		log.Debug().Str("platform", cfg.Platform).Int("version", cfg.Version).Msg("vfs: config")
	}

	key, err := hex.DecodeString(cfg.HexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}

	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: expected %d, got %d", KeySize, len(key))
	}

	// 获取平台版本对应的解密参数
	params := getDecryptParams(cfg.Platform, cfg.Version)

	if Debug {
		log.Debug().
			Int("pageSize", params.PageSize).
			Int("iterCount", params.IterCount).
			Int("hmacSize", params.HmacSize).
			Int("reserve", params.Reserve).
			Msg("vfs: params")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	fileSize := fileInfo.Size()
	totalPages := fileSize / int64(params.PageSize)
	if fileSize%int64(params.PageSize) > 0 {
		totalPages++
	}

	// 读取 salt
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(file, salt); err != nil {
		file.Close()
		return nil, err
	}

	// 派生密钥
	encKey, macKey := deriveKeys(key, salt, params)

	wf := &WechatFile{
		file:         file,
		filePath:     path,
		fileSize:     fileSize,
		salt:         salt,
		encKey:       encKey,
		macKey:       macKey,
		hashFunc:     params.HashFunc,
		hmacSize:     params.HmacSize,
		reserve:      params.Reserve,
		pageSize:     params.PageSize,
		totalPages:   totalPages,
		platform:     cfg.Platform,
		version:      cfg.Version,
		cache:        make(map[int64][]byte),
		cacheOrder:   make([]int64, 0),
		maxCacheSize: 100, // 缓存最多 100 个页面
	}

	return wf, nil
}

// decryptPage 解密单个页面
func (wf *WechatFile) decryptPage(pageNum int64) ([]byte, error) {
	if Debug {
		log.Debug().Int64("pageNum", pageNum).Str("platform", wf.platform).Int("version", wf.version).Msg("vfs: decryptPage")
	}

	// 检查缓存
	wf.cacheMu.RLock()
	if data, ok := wf.cache[pageNum]; ok {
		wf.cacheMu.RUnlock()
		if Debug {
			log.Debug().Int64("pageNum", pageNum).Msg("vfs: cache hit")
		}
		return data, nil
	}
	wf.cacheMu.RUnlock()

	// 读取加密页面
	fileOffset := pageNum * int64(wf.pageSize)
	pageBuf := make([]byte, wf.pageSize)
	n, err := wf.file.ReadAt(pageBuf, fileOffset)
	if err != nil && err != io.EOF {
		if Debug {
			log.Debug().Err(err).Msg("vfs: ReadAt error")
		}
		return nil, err
	}
	if n == 0 {
		return nil, io.EOF
	}

	if Debug {
		log.Debug().Int("bytes", n).Int64("offset", fileOffset).Msg("vfs: read")
	}

	// 检查是否全为零
	allZeros := true
	for _, b := range pageBuf[:n] {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		// 返回零页面
		result := make([]byte, wf.pageSize)
		wf.addToCache(pageNum, result)
		return result, nil
	}

	// 解密
	// 对于第一页，前 SaltSize 字节是 salt，不参与解密
	dataOffset := 0
	if pageNum == 0 {
		dataOffset = SaltSize
	}

	// 验证 HMAC
	mac := hmac.New(wf.hashFunc, wf.macKey)
	mac.Write(pageBuf[dataOffset : wf.pageSize-wf.reserve+IVSize])
	pageNoBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(pageNoBytes, uint32(pageNum+1))
	mac.Write(pageNoBytes)
	calculatedMAC := mac.Sum(nil)

	hashMacStartOffset := wf.pageSize - wf.reserve + IVSize
	storedMAC := pageBuf[hashMacStartOffset : hashMacStartOffset+wf.hmacSize]

	if !hmac.Equal(calculatedMAC, storedMAC) {
		if Debug {
			log.Debug().
				Int64("pageNum", pageNum).
				Int("dataOffset", dataOffset).
				Int("reserve", wf.reserve).
				Int("hmacSize", wf.hmacSize).
				Int("pageSize", wf.pageSize).
				Int("hashMacStartOffset", hashMacStartOffset).
				Hex("calculated", calculatedMAC[:min(16, len(calculatedMAC))]).
				Hex("stored", storedMAC[:min(16, len(storedMAC))]).
				Hex("pageHeader", pageBuf[:min(32, len(pageBuf))]).
				Msg("vfs: HMAC failed")
		}
		return nil, fmt.Errorf("HMAC verification failed for page %d", pageNum)
	}

	if Debug {
		log.Debug().Int64("pageNum", pageNum).Msg("vfs: HMAC OK")
	}

	// AES-CBC 解密
	iv := pageBuf[wf.pageSize-wf.reserve : wf.pageSize-wf.reserve+IVSize]
	block, err := aes.NewCipher(wf.encKey)
	if err != nil {
		return nil, err
	}
	mode := cipher.NewCBCDecrypter(block, iv)

	// 加密数据长度 = pageSize - reserve - dataOffset
	encryptedLen := wf.pageSize - wf.reserve - dataOffset
	decrypted := make([]byte, encryptedLen)
	copy(decrypted, pageBuf[dataOffset:wf.pageSize-wf.reserve])
	mode.CryptBlocks(decrypted, decrypted)

	// 构造解密后的完整页面
	result := make([]byte, wf.pageSize)

	if pageNum == 0 {
		// 第一页特殊处理：
		// 原始结构: [16字节salt][加密数据][reserve区]
		// 解密后:   [SQLite头16字节][解密数据][reserve区]
		copy(result[0:SaltSize], []byte(SQLiteHeader))
		copy(result[SaltSize:SaltSize+encryptedLen], decrypted)
		copy(result[SaltSize+encryptedLen:], pageBuf[wf.pageSize-wf.reserve:wf.pageSize])

		if Debug {
			log.Debug().
				Int("encryptedLen", encryptedLen).
				Int("reserve", wf.reserve).
				Int("pageSize", wf.pageSize).
				Hex("resultHeader", result[:min(32, len(result))]).
				Msg("vfs: Page 0 decrypted")
		}
	} else {
		// 其他页面：直接拷贝解密数据和保留区
		copy(result[0:encryptedLen], decrypted)
		copy(result[encryptedLen:], pageBuf[wf.pageSize-wf.reserve:wf.pageSize])
	}

	wf.addToCache(pageNum, result)
	return result, nil
}

// addToCache 添加页面到缓存
func (wf *WechatFile) addToCache(pageNum int64, data []byte) {
	wf.cacheMu.Lock()
	defer wf.cacheMu.Unlock()

	// 如果缓存已满，移除最旧的
	if len(wf.cacheOrder) >= wf.maxCacheSize {
		oldest := wf.cacheOrder[0]
		wf.cacheOrder = wf.cacheOrder[1:]
		delete(wf.cache, oldest)
	}

	wf.cache[pageNum] = data
	wf.cacheOrder = append(wf.cacheOrder, pageNum)
}

// read 从解密后的数据中读取指定范围
func (wf *WechatFile) read(buf []byte, offset int64) (int, error) {
	if offset >= wf.decryptedSize() {
		return 0, io.EOF
	}

	totalRead := 0
	remaining := len(buf)

	for remaining > 0 && offset < wf.decryptedSize() {
		pageNum := offset / int64(wf.pageSize)
		pageOffset := int(offset % int64(wf.pageSize))

		pageData, err := wf.decryptPage(pageNum)
		if err != nil {
			if totalRead > 0 {
				return totalRead, nil
			}
			return 0, err
		}

		// 计算本页可读取的字节数
		canRead := wf.pageSize - pageOffset
		if canRead > remaining {
			canRead = remaining
		}
		if int64(pageOffset+canRead) > wf.decryptedSize()-pageNum*int64(wf.pageSize) {
			canRead = int(wf.decryptedSize() - pageNum*int64(wf.pageSize) - int64(pageOffset))
		}

		copy(buf[totalRead:], pageData[pageOffset:pageOffset+canRead])
		totalRead += canRead
		remaining -= canRead
		offset += int64(canRead)
	}

	return totalRead, nil
}

// decryptedSize 返回解密后的文件大小
func (wf *WechatFile) decryptedSize() int64 {
	return wf.fileSize
}

// close 关闭文件
func (wf *WechatFile) close() error {
	return wf.file.Close()
}

//export goWechatFileOpen
func goWechatFileOpen(name *C.char, flags C.int) C.int {
	path := C.GoString(name)

	wf, err := openWechatFile(path)
	if err != nil {
		return -1
	}

	filesMu.Lock()
	id := nextFileID
	nextFileID++
	files[id] = wf
	filesMu.Unlock()

	return C.int(id)
}

//export goWechatFileRead
func goWechatFileRead(fileId C.int, buf unsafe.Pointer, amt C.int, offset C.longlong) C.int {
	filesMu.RLock()
	wf, ok := files[int(fileId)]
	filesMu.RUnlock()

	if !ok {
		return C.SQLITE_IOERR_READ
	}

	goBuf := (*[1 << 30]byte)(buf)[:amt:amt]
	n, err := wf.read(goBuf, int64(offset))

	if err != nil && err != io.EOF {
		return C.SQLITE_IOERR_READ
	}

	if n < int(amt) {
		// 用零填充剩余部分
		for i := n; i < int(amt); i++ {
			goBuf[i] = 0
		}
		if n == 0 {
			return C.SQLITE_IOERR_SHORT_READ
		}
	}

	return C.SQLITE_OK
}

//export goWechatFileClose
func goWechatFileClose(fileId C.int) C.int {
	filesMu.Lock()
	wf, ok := files[int(fileId)]
	if ok {
		delete(files, int(fileId))
	}
	filesMu.Unlock()

	if !ok {
		return C.SQLITE_OK
	}

	if err := wf.close(); err != nil {
		return C.SQLITE_IOERR_CLOSE
	}
	return C.SQLITE_OK
}

//export goWechatFileSize
func goWechatFileSize(fileId C.int) C.longlong {
	filesMu.RLock()
	wf, ok := files[int(fileId)]
	filesMu.RUnlock()

	if !ok {
		return 0
	}

	return C.longlong(wf.decryptedSize())
}