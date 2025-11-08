# WeChat v4 图片解密修复文档

## 问题背景

### 现象
在使用 chatlog 访问微信聊天记录中的图片时，HTTP API 返回的图片 URL 无法正常显示，浏览器下载到的是加密的 `.dat` 文件而非真实图片格式（JPG/PNG/GIF 等）。

日志显示错误：
```
DBG Failed to decrypt dat file error="unknown image type after decryption"
```

### 环境
- 微信版本：4.0.3.80 (macOS)
- chatlog 版本：commit 608c574
- 数据源：外接硬盘 `/Volumes/T7-red/com.tencent.xinWeChat/...`
- 图片格式：V4Format2 (`07 08 56 32`) with AES-ECB + XOR 双重加密

### 根本原因
微信 v4.x 版本使用了复杂的图片加密方案：
1. **AES-ECB 加密**：前 1KB 左右使用 AES-128-ECB 加密，密钥为 `img_key`（16字节）
2. **XOR 加密**：文件尾部使用单字节 XOR 加密，密钥因版本而异（0xDF, 0xE5 等）
3. **异步扫描**：原代码中 XOR key 扫描是异步的 (`go dat2img.ScanAndSetXorKey`)，导致 HTTP 服务启动后立即访问图片时密钥未就绪

## 成功原因分析

### 技术突破点

#### 1. 同步 XOR Key 扫描
**修改文件：** `internal/chatlog/manager.go:113`

**改动前：**
```go
if m.ctx.Version == 4 {
    dat2img.SetAesKey(m.ctx.ImgKey)
    go dat2img.ScanAndSetXorKey(m.ctx.DataDir)  // 异步扫描
}
```

**改动后：**
```go
if m.ctx.Version == 4 {
    dat2img.SetAesKey(m.ctx.ImgKey)
    dat2img.ScanAndSetXorKey(m.ctx.DataDir)  // 同步扫描，阻塞至完成
}
```

**效果：** 确保 HTTP 服务启动时，XOR key 已经扫描完成并设置好，避免请求时密钥未就绪导致的解密失败。

#### 2. 多 XOR Key 自动尝试与智能选择
**修改文件：** `pkg/util/dat2img/dat2img.go:255-316`

**核心策略：**
```go
// 尝试多个 XOR key
xorKeys := []byte{V4XorKey, 0xDF, 0xE5}  // 扫描值、社区默认值、实测值
var bestResult []byte
var bestExt string

for _, xorKey := range xorKeys {
    // 尝试解密
    xorDecrypted := make([]byte, len(xorData))
    for i := range xorData {
        xorDecrypted[i] = xorData[i] ^ xorKey
    }

    testResult := append(result, xorDecrypted...)

    // 验证是否产生有效图片
    for _, format := range Formats {
        if bytes.Equal(testResult[:len(format.Header)], format.Header) {
            // 优先选择解密出最大文件的结果（更完整）
            if bestResult == nil || len(testResult) > len(bestResult) {
                bestResult = testResult
                bestExt = format.Ext
            }
        }
    }
}
```

**创新点：**
- **兼容性**：自动尝试多个 XOR key，覆盖不同微信版本
- **智能性**：通过验证图片 magic bytes (文件头) 判断解密是否成功
- **完整性**：选择能解密出最大文件的 key（更完整的图片）
- **鲁棒性**：即使扫描 XOR key 失败，仍能通过固定 key 列表尝试解密

### 关键测试验证

**测试代码：** `test_direct.go`

模拟了完整的服务启动流程：
```go
// 1. Hex decode img_key
decoded, _ := hex.DecodeString("38386636353935396131366365383239")
// Result: 38386636353935396131366365383239 (16 bytes)

// 2. 扫描 XOR key
xorKey, _ := dat2img.ScanAndSetXorKey(dataDir)
// Result: 0xe5

// 3. 解密测试文件
imgData, ext, err := dat2img.Dat2Image(data)
// Result: SUCCESS! Type: jpg, Size: 438885 bytes
```

**对比测试结果：**
| 配置 | XOR Key | 解密结果 | 文件大小 | 状态 |
|------|---------|----------|----------|------|
| img_key (decoded) + 0xE5 | 0xE5 | ✅ JPG | 429 KB | **最优** |
| img_key (hex string) + 0xE5 | 0xE5 | ❌ | - | 失败 |
| img_key (decoded) + 0xDF | 0xDF | ✅ JPG | 42 KB | 部分 |
| 默认 key + 0xDF | 0xDF | ❌ | - | 失败 |

**结论：** 正确的 `img_key` (hex decoded) + 扫描到的 XOR key (0xE5) 能解密出完整图片。

## 与现有代码的关系

### PR #192 的贡献
代码库已包含 PR #192 的完整实现：

✅ **已实现功能：**
- `AesKeyValidator` (`pkg/util/dat2img/imgkey.go`): 图片密钥验证器
- `Wxam2pic` (`pkg/util/dat2img/wxgf.go`): WXGF 格式解析与转换
- V4Format1/V4Format2 支持 (`pkg/util/dat2img/dat2img.go`)
- `img_key` 配置字段 (`internal/chatlog/conf/`)
- FFmpeg 集成支持

### 本次改动的补充
本次改动 (commit 8e8edd0) 在 PR #192 基础上进一步增强：

**新增能力：**
1. **时序优化**：XOR key 扫描从异步改为同步，解决竞态条件
2. **容错机制**：多 XOR key fallback 策略，提升兼容性
3. **智能选择**：基于解密结果完整性自动选择最佳 key

**不冲突反而协同：**
- PR #192 提供了底层的 AES/XOR 解密算法和 WXGF 支持
- 本次改动优化了算法的调用时机和 key 选择策略
- 两者结合形成完整的图片解密解决方案

## 参考资料

### 关键 Issues
1. **[Issue #139](https://github.com/sjzar/chatlog/issues/139)** - 社区用户报告图片解密失败
   - 提供了 Python 解密脚本参考
   - 确认了固定 XOR key `0xDF` 的有效性
   - 提到了文件头 `0xDF, 0xF8` 的特征

2. **[Issue #119, #68, #93, #88](https://github.com/sjzar/chatlog/pull/192)** - 其他相关加密图片问题

### 关键 PR
- **[PR #192](https://github.com/sjzar/chatlog/pull/192)** - "get image aes key & parse wxgf format"
  - 实现了 `img_key` 提取功能
  - 添加了 WXGF 格式解析
  - 提供了 AES key 验证机制
  - 合并时间：2025-08-17

### 技术参考
- [wechat-dump-rs](https://github.com/0xlane/wechat-dump-rs) - Rust 实现的微信数据提取
- [PyWxDump](https://github.com/xaoyaoo/PyWxDump) - Python 实现的微信数据导出

## 总结

### 成功关键因素
1. **完整的密钥配置**：正确提取并配置 `img_key` (hex string → 16 bytes)
2. **XOR key 扫描**：自动从样本文件中计算 XOR key（本例为 0xE5）
3. **时序保证**：同步扫描确保服务启动时密钥就绪
4. **容错策略**：多 XOR key 尝试提升兼容性
5. **智能选择**：基于解密结果质量选择最佳 key

### 适用场景
- ✅ 微信 4.0.x 版本（macOS/Windows）
- ✅ V4Format1 (`07 08 56 31`) 和 V4Format2 (`07 08 56 32`)
- ✅ 混合加密（AES-ECB + XOR）
- ✅ WXGF 格式动图
- ✅ 多种 XOR key 变体（0xDF, 0xE5, 0x37 等）

### 未来优化方向
1. 缓存 XOR key 扫描结果，避免每次启动重复扫描
2. 支持更多图片格式（WebP, HEIF 等）
3. 提供 XOR key 手动配置选项
4. 优化扫描算法性能（并行扫描、样本数量限制）

---

**文档版本：** 1.0
**最后更新：** 2025-11-08
**相关 Commit：** 8e8edd0 "fix(decrypt): improve WeChat v4 image decryption reliability"
