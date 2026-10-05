# pbr-texture-pipeline-go

一套完全在进程内运行的贴图派生与通道打包流水线：从高度/位移贴图生成切线空间法线贴图，把多张单通道数据贴图合并成运行时贴图，并基于内容寻址缓存做增量加工。不依赖任何外部服务，结论与错误均可在本地稳定复现。

## 构建与测试

```sh
go build ./...
go test ./... -race
go run ./cmd/pbrtp   # 演示：合成高度图 -> 法线 + ORM 打包，第二轮全部命中缓存
```

## 核心概念

所有贴图在内存中表示为 `tex.Image`：float32、行优先、通道交错，并携带色彩空间（`Linear`/`SRGB`）与方向元信息（`Direction`）。

### 法线贴图约定（`NormalConfig`）

- `Method`：梯度算子。`GradientSobel`（3x3 Sobel，抗噪）或 `GradientCentral`（中心差分，锐利）。
- `Strength`：梯度缩放系数，必须 >= 0；为 0 时输出平直法线 (0,0,1)。
- `YUp`：输出绿通道约定。`true` = OpenGL（绿通道指向 +v），`false` = DirectX（绿通道指向 −v）。两种输出互为绿通道镜像。
- 输入高度图的方向元信息参与计算：`OriginTopLeft` 表示第 0 行在图像顶部，此时 +v 方向对应行号减小。
- 输出为 3 通道 `Linear` 贴图，纹素按 `n*0.5+0.5` 编码；`DecodeNormal` 解码回单位向量。每个纹素解码后都是单位长度、z 分量朝外。
- 边缘采样采用回绕（wrap）寻址：周期性的高度场生成的法线图可以无缝平铺，最边缘也不会出现接缝。

### 通道打包规则（`PackConfig`）

- `Sources[i]` 唯一决定输出通道 `i` 的来源（`ImageIndex` + `Channel`），映射顺序完全由配置决定。
- `OutChannels` 为输出通道数；超出 `len(Sources)` 的通道写入 `FillValue`。
- 每个输出（像素, 通道）只被写入一次、只来自一个来源，因此任意并发度与处理顺序下结果逐像素一致，不会通道错位或相互覆盖。
- 打包结果属于数据贴图，输出恒为 `Linear`，不做伽马变换。

### 色彩空间

- `SRGBToLinear` / `LinearToSRGB` 实现 IEC 61966-2-1 分段曲线。
- `ConvertSpace(img, target, isColor)`：只有 `isColor=true` 的颜色贴图才会做伽马编解码；数据贴图（法线、高度、ORM 等）传 `false`，数值绝不被改动。转换不修改原图。
- 同一内容分别存于 Linear 与 sRGB 时，归一到 Linear 后像素一致（误差 < 1e-6）；单次往返误差 < 1e-5。

### 输入校验

`ValidateInputs` 在处理开始前拒绝整批输入，错误为 `*ValidationError`，含 `Kind`（`ErrSizeMismatch` / `ErrChannelMismatch` / `ErrDirectionConflict` / `ErrInvalidConfig`）与涉事输入下标 `Inputs`，可区分、可定位；校验失败不会产出任何部分可用的贴图。

### 增量加工与缓存

- 缓存键 = `Key(op, 内容哈希列表, 配置规范串)`，并混入 `AlgorithmVersion`。键只由输入内容与相关配置决定：内容或配置变化 → 重新生成；算法升级（提升 `AlgorithmVersion`）→ 全部失效；其余情况复用。
- 增量产出的贴图集与全量重建逐字节一致（测试覆盖）。每个 `Result.FromCache` 标明该产物是命中复用还是重新生成。
- 缓存条目带魔数、键与 SHA-256 校验和；损坏或写入不完整的条目不会被当作有效结果返回，并会被清除。写入采用临时文件 + rename，崩溃不会留下半成品。

### 并发与取消

- `Pipeline` 以信号量限制并发度（默认 4），同时限制在途任务数与峰值内存；单个超大图或慢任务占用的 worker 数有上界。
- `Run` 接受 `context.Context`：取消或超时后任务立即报错返回，且不会写入缓存，不会留下污染后续请求的中间状态。

## 目录结构

| 文件 | 职责 |
| --- | --- |
| `tex/image.go` | 图像类型、像素访问、回绕采样 |
| `tex/normal.go` | 高度/位移 → 切线空间法线 |
| `tex/pack.go` | 多源通道打包 |
| `tex/colorspace.go` | sRGB/Linear 编解码与转换 |
| `tex/validate.go` | 尺寸/通道/方向一致性校验 |
| `tex/cache.go` | 内容寻址缓存、完整性校验、原子写入 |
| `tex/serialize.go` | 图像的确定性二进制编码 |
| `tex/pipeline.go` | 并发调度、增量复用、取消 |
| `cmd/pbrtp` | 进程内演示程序 |
