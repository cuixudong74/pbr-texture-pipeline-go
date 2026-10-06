# pbr-texture-pipeline-go

一套纯进程内的贴图派生与通道打包流水线：从高度/位移贴图生成切线空间法线贴图，
把多张单通道数据贴图合并为运行时贴图（如 ORM），并通过内容寻址缓存支持增量加工。
不依赖任何外部服务，所有结论与错误均可在本地稳定复现。

## 构建与测试

```sh
go build ./...
go test ./...
go run ./cmd/texturegen -out out -cache cache   # 端到端示例；运行两次可见增量复用
```

## 核心概念

### Texture

`pipeline.Texture` 是进程内图像：`float32` 像素、行主序、通道交错，
携带 `ColorSpace`（`Linear` / `SRGB`）与 `Orientation`（`OrientationYDown` /
`OrientationYUp`）元信息。所有计算都在线性空间进行。

### 法线贴图生成（`GenerateNormal` / `NormalConfig`）

| 配置项 | 含义 |
| --- | --- |
| `Strength` | 梯度缩放系数，越大法线倾斜越明显 |
| `Filter` | `CentralDifference`（2 采样中心差分）或 `Sobel`（3x3 Sobel） |
| `Convention` | `OpenGLNormals`（Y+，绿通道朝上）或 `DirectXNormals`（Y-，绿通道朝下） |
| `SourceChannel` | 高度/位移值所在的源通道 |
| `WrapHorizontal` / `WrapVertical` | 边缘是否环绕采样 |

约定与保证：

- 图像行向下为 +y。OpenGL 约定下，高度向图像底部上升时法线偏向 +Y（绿通道偏亮）；
  DirectX 约定为其镜像（仅绿通道翻转，其余通道一致）。
- 输出为 3 通道线性贴图，单位向量编码到 `[0,1]`（`n*0.5+0.5`），z 恒为正。
- 开启环绕后，边缘像素的梯度取自对侧边缘，输出在平铺时与 3x3 平铺参考
  逐像素一致，不会产生可见接缝（有测试保证）。关闭环绕则边缘做钳制采样。

### 通道打包（`Pack` / `PackConfig`）

`PackConfig.Sources` 按输出通道顺序（R、G、B、A）逐一给出唯一来源：

```go
PackConfig{
    Sources: []ChannelSource{
        {Input: 0, Channel: 0},            // R <= inputs[0].ch0（如 AO）
        {Input: 1, Channel: 0},            // G <= inputs[1].ch0（如 Roughness）
        {Input: 2, Channel: 0, Invert: true}, // B <= 1 - inputs[2].ch0
        {Input: -1},                       // A <= Defaults[3]（常量填充）
    },
    Defaults: []float32{0, 0, 0, 1},
}
```

- 合并顺序与通道映射完全由配置决定；每个输出通道只被写入一次，
  任意并发度与处理顺序下结果逐像素一致。
- 打包输出恒为 `Linear`：数据贴图不会被当作颜色做伽马变换。
- 任何校验失败（尺寸不一致、通道不足、方向冲突、非法引用）都会在
  写出任何结果之前返回 `*ValidationError`，不会产出部分可用的贴图。

### 色彩空间

- `DecodeToLinear`：仅对声明为 `SRGB` 的贴图做 sRGB→linear 解码；
  alpha 通道永不做伽马变换；`Linear` 贴图原样通过。
- `EncodeFromLinear(t, target)`：目标为 `SRGB` 时编码，目标为 `Linear` 时原样通过。
- 同一内容分别以 linear 与 sRGB 存储时，归一化（解码到 linear）后像素一致；
  单次往返误差在 1e-5 以内。

### 输入校验（`ValidateInputs` / `ValidationError`）

在处理开始前统一校验所有输入的尺寸、通道数与方向元信息。
`ValidationError.Kind` 可区分失败原因，且 `Inputs` 字段定位到具体输入下标：

| Kind | 触发条件 |
| --- | --- |
| `ErrSizeMismatch` | 输入尺寸不一致 |
| `ErrChannelInsufficient` | 配置引用了输入不具备的通道 |
| `ErrOrientationConflict` | 输入方向元信息冲突 |
| `ErrColorSpaceConflict` | 预留：色彩空间冲突 |
| `ErrInvalidConfig` | 空输入、非法通道引用等配置错误 |

### 增量加工与缓存（`Pipeline` / `Cache`）

- 缓存键 = `AlgorithmVersion` + 操作名 + 配置的规范 JSON + 各输入内容哈希，
  只由输入内容与相关配置决定。配置或算法版本变化会使对应结果整体失效。
- 增量运行只重新生成内容或配置真正变化的产物，其余命中缓存；
  每个 `Artifact.Reused` 标明该产物是命中复用还是重新生成。
  序列化是确定性的，增量产出的贴图集与全量重建逐字节一致。
- 缓存写入先落临时文件再原子改名；读取时校验载荷 SHA-256，
  损坏或写入不完整的条目会被删除并按未命中处理，绝不作为有效结果返回。

### 并发、内存与取消

- `Pipeline.MaxConcurrency` 限制同时执行的作业数（默认 4）。
- `Pipeline.MaxMemoryBytes` 限制整批作业的估计工作内存（默认 1 GiB，
  以 64 KiB 为单位记账）。单个超过预算的作业会独占全部额度单独执行，不会死锁。
- `Run` 接受 `context.Context`：取消或超时后返回 `ctx.Err()`，
  已完成写入因原子改名而不会留下半成品，后续请求不受污染。

## 目录结构

```
pipeline/            核心库（纹理、法线、打包、色彩空间、校验、缓存、调度）
cmd/texturegen/      端到端示例：进程内生成样本图，输出 PNG 预览与复用报告
```
