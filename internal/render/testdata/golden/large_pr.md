<!-- shale-card -->
## 🧾 Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 · session `a1b2c3` · 14 prompts · transcript hash `sha256:4be91d03…`*

### Completion
> Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (201) — 0 seen in agent sessions, 201 not
| File | Agent session | Notes |
|---|---|---|
| `pkg/gen/file_000.go` | ⚠️ none |  |
| `pkg/gen/file_001.go` | ⚠️ none |  |
| `pkg/gen/file_002.go` | ⚠️ none |  |
| `pkg/gen/file_003.go` | ⚠️ none |  |
| `pkg/gen/file_004.go` | ⚠️ none |  |
| `pkg/gen/file_005.go` | ⚠️ none |  |
| `pkg/gen/file_006.go` | ⚠️ none |  |
| `pkg/gen/file_007.go` | ⚠️ none |  |
| `pkg/gen/file_008.go` | ⚠️ none |  |
| `pkg/gen/file_009.go` | ⚠️ none |  |
| `pkg/gen/file_010.go` | ⚠️ none |  |
| `pkg/gen/file_011.go` | ⚠️ none |  |
| `pkg/gen/file_012.go` | ⚠️ none |  |
| `pkg/gen/file_013.go` | ⚠️ none |  |
| `pkg/gen/file_014.go` | ⚠️ none |  |
| `pkg/gen/file_015.go` | ⚠️ none |  |
| `pkg/gen/file_016.go` | ⚠️ none |  |
| `pkg/gen/file_017.go` | ⚠️ none |  |
| `pkg/gen/file_018.go` | ⚠️ none |  |
| `pkg/gen/file_019.go` | ⚠️ none |  |
| `pkg/gen/file_020.go` | ⚠️ none |  |
| `pkg/gen/file_021.go` | ⚠️ none |  |
| `pkg/gen/file_022.go` | ⚠️ none |  |
| `pkg/gen/file_023.go` | ⚠️ none |  |
| `pkg/gen/file_024.go` | ⚠️ none |  |
| `pkg/gen/file_025.go` | ⚠️ none |  |
| `pkg/gen/file_026.go` | ⚠️ none |  |
| `pkg/gen/file_027.go` | ⚠️ none |  |
| `pkg/gen/file_028.go` | ⚠️ none |  |
| `pkg/gen/file_029.go` | ⚠️ none |  |
| `pkg/gen/file_030.go` | ⚠️ none |  |
| `pkg/gen/file_031.go` | ⚠️ none |  |
| `pkg/gen/file_032.go` | ⚠️ none |  |
| `pkg/gen/file_033.go` | ⚠️ none |  |
| `pkg/gen/file_034.go` | ⚠️ none |  |
| `pkg/gen/file_035.go` | ⚠️ none |  |
| `pkg/gen/file_036.go` | ⚠️ none |  |
| `pkg/gen/file_037.go` | ⚠️ none |  |
| `pkg/gen/file_038.go` | ⚠️ none |  |
| `pkg/gen/file_039.go` | ⚠️ none |  |
| `pkg/gen/file_040.go` | ⚠️ none |  |
| `pkg/gen/file_041.go` | ⚠️ none |  |
| `pkg/gen/file_042.go` | ⚠️ none |  |
| `pkg/gen/file_043.go` | ⚠️ none |  |
| `pkg/gen/file_044.go` | ⚠️ none |  |
| `pkg/gen/file_045.go` | ⚠️ none |  |
| `pkg/gen/file_046.go` | ⚠️ none |  |
| `pkg/gen/file_047.go` | ⚠️ none |  |
| `pkg/gen/file_048.go` | ⚠️ none |  |
| `pkg/gen/file_049.go` | ⚠️ none |  |
| `pkg/gen/file_050.go` | ⚠️ none |  |
| `pkg/gen/file_051.go` | ⚠️ none |  |
| `pkg/gen/file_052.go` | ⚠️ none |  |
| `pkg/gen/file_053.go` | ⚠️ none |  |
| `pkg/gen/file_054.go` | ⚠️ none |  |
| `pkg/gen/file_055.go` | ⚠️ none |  |
| `pkg/gen/file_056.go` | ⚠️ none |  |
| `pkg/gen/file_057.go` | ⚠️ none |  |
| `pkg/gen/file_058.go` | ⚠️ none |  |
| `pkg/gen/file_059.go` | ⚠️ none |  |
| `pkg/gen/file_060.go` | ⚠️ none |  |
| `pkg/gen/file_061.go` | ⚠️ none |  |
| `pkg/gen/file_062.go` | ⚠️ none |  |
| `pkg/gen/file_063.go` | ⚠️ none |  |
| `pkg/gen/file_064.go` | ⚠️ none |  |
| `pkg/gen/file_065.go` | ⚠️ none |  |
| `pkg/gen/file_066.go` | ⚠️ none |  |
| `pkg/gen/file_067.go` | ⚠️ none |  |
| `pkg/gen/file_068.go` | ⚠️ none |  |
| `pkg/gen/file_069.go` | ⚠️ none |  |
| `pkg/gen/file_070.go` | ⚠️ none |  |
| `pkg/gen/file_071.go` | ⚠️ none |  |
| `pkg/gen/file_072.go` | ⚠️ none |  |
| `pkg/gen/file_073.go` | ⚠️ none |  |
| `pkg/gen/file_074.go` | ⚠️ none |  |
| `pkg/gen/file_075.go` | ⚠️ none |  |
| `pkg/gen/file_076.go` | ⚠️ none |  |
| `pkg/gen/file_077.go` | ⚠️ none |  |
| `pkg/gen/file_078.go` | ⚠️ none |  |
| `pkg/gen/file_079.go` | ⚠️ none |  |
| `pkg/gen/file_080.go` | ⚠️ none |  |
| `pkg/gen/file_081.go` | ⚠️ none |  |
| `pkg/gen/file_082.go` | ⚠️ none |  |
| `pkg/gen/file_083.go` | ⚠️ none |  |
| `pkg/gen/file_084.go` | ⚠️ none |  |
| `pkg/gen/file_085.go` | ⚠️ none |  |
| `pkg/gen/file_086.go` | ⚠️ none |  |
| `pkg/gen/file_087.go` | ⚠️ none |  |
| `pkg/gen/file_088.go` | ⚠️ none |  |
| `pkg/gen/file_089.go` | ⚠️ none |  |
| `pkg/gen/file_090.go` | ⚠️ none |  |
| `pkg/gen/file_091.go` | ⚠️ none |  |
| `pkg/gen/file_092.go` | ⚠️ none |  |
| `pkg/gen/file_093.go` | ⚠️ none |  |
| `pkg/gen/file_094.go` | ⚠️ none |  |
| `pkg/gen/file_095.go` | ⚠️ none |  |
| `pkg/gen/file_096.go` | ⚠️ none |  |
| `pkg/gen/file_097.go` | ⚠️ none |  |
| `pkg/gen/file_098.go` | ⚠️ none |  |
| `pkg/gen/file_099.go` | ⚠️ none |  |
| `pkg/gen/file_100.go` | ⚠️ none |  |
| `pkg/gen/file_101.go` | ⚠️ none |  |
| `pkg/gen/file_102.go` | ⚠️ none |  |
| `pkg/gen/file_103.go` | ⚠️ none |  |
| `pkg/gen/file_104.go` | ⚠️ none |  |
| `pkg/gen/file_105.go` | ⚠️ none |  |
| `pkg/gen/file_106.go` | ⚠️ none |  |
| `pkg/gen/file_107.go` | ⚠️ none |  |
| `pkg/gen/file_108.go` | ⚠️ none |  |
| `pkg/gen/file_109.go` | ⚠️ none |  |
| `pkg/gen/file_110.go` | ⚠️ none |  |
| `pkg/gen/file_111.go` | ⚠️ none |  |
| `pkg/gen/file_112.go` | ⚠️ none |  |
| `pkg/gen/file_113.go` | ⚠️ none |  |
| `pkg/gen/file_114.go` | ⚠️ none |  |
| `pkg/gen/file_115.go` | ⚠️ none |  |
| `pkg/gen/file_116.go` | ⚠️ none |  |
| `pkg/gen/file_117.go` | ⚠️ none |  |
| `pkg/gen/file_118.go` | ⚠️ none |  |
| `pkg/gen/file_119.go` | ⚠️ none |  |
| `pkg/gen/file_120.go` | ⚠️ none |  |
| `pkg/gen/file_121.go` | ⚠️ none |  |
| `pkg/gen/file_122.go` | ⚠️ none |  |
| `pkg/gen/file_123.go` | ⚠️ none |  |
| `pkg/gen/file_124.go` | ⚠️ none |  |
| `pkg/gen/file_125.go` | ⚠️ none |  |
| `pkg/gen/file_126.go` | ⚠️ none |  |
| `pkg/gen/file_127.go` | ⚠️ none |  |
| `pkg/gen/file_128.go` | ⚠️ none |  |
| `pkg/gen/file_129.go` | ⚠️ none |  |
| `pkg/gen/file_130.go` | ⚠️ none |  |
| `pkg/gen/file_131.go` | ⚠️ none |  |
| `pkg/gen/file_132.go` | ⚠️ none |  |
| `pkg/gen/file_133.go` | ⚠️ none |  |
| `pkg/gen/file_134.go` | ⚠️ none |  |
| `pkg/gen/file_135.go` | ⚠️ none |  |
| `pkg/gen/file_136.go` | ⚠️ none |  |
| `pkg/gen/file_137.go` | ⚠️ none |  |
| `pkg/gen/file_138.go` | ⚠️ none |  |
| `pkg/gen/file_139.go` | ⚠️ none |  |
| `pkg/gen/file_140.go` | ⚠️ none |  |
| `pkg/gen/file_141.go` | ⚠️ none |  |
| `pkg/gen/file_142.go` | ⚠️ none |  |
| `pkg/gen/file_143.go` | ⚠️ none |  |
| `pkg/gen/file_144.go` | ⚠️ none |  |
| `pkg/gen/file_145.go` | ⚠️ none |  |
| `pkg/gen/file_146.go` | ⚠️ none |  |
| `pkg/gen/file_147.go` | ⚠️ none |  |
| `pkg/gen/file_148.go` | ⚠️ none |  |
| `pkg/gen/file_149.go` | ⚠️ none |  |
| `pkg/gen/file_150.go` | ⚠️ none |  |
| `pkg/gen/file_151.go` | ⚠️ none |  |
| `pkg/gen/file_152.go` | ⚠️ none |  |
| `pkg/gen/file_153.go` | ⚠️ none |  |
| `pkg/gen/file_154.go` | ⚠️ none |  |
| `pkg/gen/file_155.go` | ⚠️ none |  |
| `pkg/gen/file_156.go` | ⚠️ none |  |
| `pkg/gen/file_157.go` | ⚠️ none |  |
| `pkg/gen/file_158.go` | ⚠️ none |  |
| `pkg/gen/file_159.go` | ⚠️ none |  |
| `pkg/gen/file_160.go` | ⚠️ none |  |
| `pkg/gen/file_161.go` | ⚠️ none |  |
| `pkg/gen/file_162.go` | ⚠️ none |  |
| `pkg/gen/file_163.go` | ⚠️ none |  |
| `pkg/gen/file_164.go` | ⚠️ none |  |
| `pkg/gen/file_165.go` | ⚠️ none |  |
| `pkg/gen/file_166.go` | ⚠️ none |  |
| `pkg/gen/file_167.go` | ⚠️ none |  |
| `pkg/gen/file_168.go` | ⚠️ none |  |
| `pkg/gen/file_169.go` | ⚠️ none |  |
| `pkg/gen/file_170.go` | ⚠️ none |  |
| `pkg/gen/file_171.go` | ⚠️ none |  |
| `pkg/gen/file_172.go` | ⚠️ none |  |
| `pkg/gen/file_173.go` | ⚠️ none |  |
| `pkg/gen/file_174.go` | ⚠️ none |  |
| `pkg/gen/file_175.go` | ⚠️ none |  |
| `pkg/gen/file_176.go` | ⚠️ none |  |
| `pkg/gen/file_177.go` | ⚠️ none |  |
| `pkg/gen/file_178.go` | ⚠️ none |  |
| `pkg/gen/file_179.go` | ⚠️ none |  |
| `pkg/gen/file_180.go` | ⚠️ none |  |
| `pkg/gen/file_181.go` | ⚠️ none |  |
| `pkg/gen/file_182.go` | ⚠️ none |  |
| `pkg/gen/file_183.go` | ⚠️ none |  |
| `pkg/gen/file_184.go` | ⚠️ none |  |
| `pkg/gen/file_185.go` | ⚠️ none |  |
| `pkg/gen/file_186.go` | ⚠️ none |  |
| `pkg/gen/file_187.go` | ⚠️ none |  |
| `pkg/gen/file_188.go` | ⚠️ none |  |
| `pkg/gen/file_189.go` | ⚠️ none |  |
| `pkg/gen/file_190.go` | ⚠️ none |  |
| `pkg/gen/file_191.go` | ⚠️ none |  |
| `pkg/gen/file_192.go` | ⚠️ none |  |
| `pkg/gen/file_193.go` | ⚠️ none |  |
| `pkg/gen/file_194.go` | ⚠️ none |  |
| `pkg/gen/file_195.go` | ⚠️ none |  |
| `pkg/gen/file_196.go` | ⚠️ none |  |
| `pkg/gen/file_197.go` | ⚠️ none |  |
| `pkg/gen/file_198.go` | ⚠️ none |  |
| `pkg/gen/file_199.go` | ⚠️ none |  |
| `.github/workflows/deploy.yml` | ⚠️ none | **sensitive path: CI config** |

Files grouped by directory:

| Directory | Files |
|---|---|
| `.github/` | 1 |
| `pkg/` | 200 |

<details><summary>Full file list</summary>

| File | Agent session | Notes |
|---|---|---|
| `pkg/gen/file_000.go` | ⚠️ none |  |
| `pkg/gen/file_001.go` | ⚠️ none |  |
| `pkg/gen/file_002.go` | ⚠️ none |  |
| `pkg/gen/file_003.go` | ⚠️ none |  |
| `pkg/gen/file_004.go` | ⚠️ none |  |
| `pkg/gen/file_005.go` | ⚠️ none |  |
| `pkg/gen/file_006.go` | ⚠️ none |  |
| `pkg/gen/file_007.go` | ⚠️ none |  |
| `pkg/gen/file_008.go` | ⚠️ none |  |
| `pkg/gen/file_009.go` | ⚠️ none |  |
| `pkg/gen/file_010.go` | ⚠️ none |  |
| `pkg/gen/file_011.go` | ⚠️ none |  |
| `pkg/gen/file_012.go` | ⚠️ none |  |
| `pkg/gen/file_013.go` | ⚠️ none |  |
| `pkg/gen/file_014.go` | ⚠️ none |  |
| `pkg/gen/file_015.go` | ⚠️ none |  |
| `pkg/gen/file_016.go` | ⚠️ none |  |
| `pkg/gen/file_017.go` | ⚠️ none |  |
| `pkg/gen/file_018.go` | ⚠️ none |  |
| `pkg/gen/file_019.go` | ⚠️ none |  |
| `pkg/gen/file_020.go` | ⚠️ none |  |
| `pkg/gen/file_021.go` | ⚠️ none |  |
| `pkg/gen/file_022.go` | ⚠️ none |  |
| `pkg/gen/file_023.go` | ⚠️ none |  |
| `pkg/gen/file_024.go` | ⚠️ none |  |
| `pkg/gen/file_025.go` | ⚠️ none |  |
| `pkg/gen/file_026.go` | ⚠️ none |  |
| `pkg/gen/file_027.go` | ⚠️ none |  |
| `pkg/gen/file_028.go` | ⚠️ none |  |
| `pkg/gen/file_029.go` | ⚠️ none |  |
| `pkg/gen/file_030.go` | ⚠️ none |  |
| `pkg/gen/file_031.go` | ⚠️ none |  |
| `pkg/gen/file_032.go` | ⚠️ none |  |
| `pkg/gen/file_033.go` | ⚠️ none |  |
| `pkg/gen/file_034.go` | ⚠️ none |  |
| `pkg/gen/file_035.go` | ⚠️ none |  |
| `pkg/gen/file_036.go` | ⚠️ none |  |
| `pkg/gen/file_037.go` | ⚠️ none |  |
| `pkg/gen/file_038.go` | ⚠️ none |  |
| `pkg/gen/file_039.go` | ⚠️ none |  |
| `pkg/gen/file_040.go` | ⚠️ none |  |
| `pkg/gen/file_041.go` | ⚠️ none |  |
| `pkg/gen/file_042.go` | ⚠️ none |  |
| `pkg/gen/file_043.go` | ⚠️ none |  |
| `pkg/gen/file_044.go` | ⚠️ none |  |
| `pkg/gen/file_045.go` | ⚠️ none |  |
| `pkg/gen/file_046.go` | ⚠️ none |  |
| `pkg/gen/file_047.go` | ⚠️ none |  |
| `pkg/gen/file_048.go` | ⚠️ none |  |
| `pkg/gen/file_049.go` | ⚠️ none |  |
| `pkg/gen/file_050.go` | ⚠️ none |  |
| `pkg/gen/file_051.go` | ⚠️ none |  |
| `pkg/gen/file_052.go` | ⚠️ none |  |
| `pkg/gen/file_053.go` | ⚠️ none |  |
| `pkg/gen/file_054.go` | ⚠️ none |  |
| `pkg/gen/file_055.go` | ⚠️ none |  |
| `pkg/gen/file_056.go` | ⚠️ none |  |
| `pkg/gen/file_057.go` | ⚠️ none |  |
| `pkg/gen/file_058.go` | ⚠️ none |  |
| `pkg/gen/file_059.go` | ⚠️ none |  |
| `pkg/gen/file_060.go` | ⚠️ none |  |
| `pkg/gen/file_061.go` | ⚠️ none |  |
| `pkg/gen/file_062.go` | ⚠️ none |  |
| `pkg/gen/file_063.go` | ⚠️ none |  |
| `pkg/gen/file_064.go` | ⚠️ none |  |
| `pkg/gen/file_065.go` | ⚠️ none |  |
| `pkg/gen/file_066.go` | ⚠️ none |  |
| `pkg/gen/file_067.go` | ⚠️ none |  |
| `pkg/gen/file_068.go` | ⚠️ none |  |
| `pkg/gen/file_069.go` | ⚠️ none |  |
| `pkg/gen/file_070.go` | ⚠️ none |  |
| `pkg/gen/file_071.go` | ⚠️ none |  |
| `pkg/gen/file_072.go` | ⚠️ none |  |
| `pkg/gen/file_073.go` | ⚠️ none |  |
| `pkg/gen/file_074.go` | ⚠️ none |  |
| `pkg/gen/file_075.go` | ⚠️ none |  |
| `pkg/gen/file_076.go` | ⚠️ none |  |
| `pkg/gen/file_077.go` | ⚠️ none |  |
| `pkg/gen/file_078.go` | ⚠️ none |  |
| `pkg/gen/file_079.go` | ⚠️ none |  |
| `pkg/gen/file_080.go` | ⚠️ none |  |
| `pkg/gen/file_081.go` | ⚠️ none |  |
| `pkg/gen/file_082.go` | ⚠️ none |  |
| `pkg/gen/file_083.go` | ⚠️ none |  |
| `pkg/gen/file_084.go` | ⚠️ none |  |
| `pkg/gen/file_085.go` | ⚠️ none |  |
| `pkg/gen/file_086.go` | ⚠️ none |  |
| `pkg/gen/file_087.go` | ⚠️ none |  |
| `pkg/gen/file_088.go` | ⚠️ none |  |
| `pkg/gen/file_089.go` | ⚠️ none |  |
| `pkg/gen/file_090.go` | ⚠️ none |  |
| `pkg/gen/file_091.go` | ⚠️ none |  |
| `pkg/gen/file_092.go` | ⚠️ none |  |
| `pkg/gen/file_093.go` | ⚠️ none |  |
| `pkg/gen/file_094.go` | ⚠️ none |  |
| `pkg/gen/file_095.go` | ⚠️ none |  |
| `pkg/gen/file_096.go` | ⚠️ none |  |
| `pkg/gen/file_097.go` | ⚠️ none |  |
| `pkg/gen/file_098.go` | ⚠️ none |  |
| `pkg/gen/file_099.go` | ⚠️ none |  |
| `pkg/gen/file_100.go` | ⚠️ none |  |
| `pkg/gen/file_101.go` | ⚠️ none |  |
| `pkg/gen/file_102.go` | ⚠️ none |  |
| `pkg/gen/file_103.go` | ⚠️ none |  |
| `pkg/gen/file_104.go` | ⚠️ none |  |
| `pkg/gen/file_105.go` | ⚠️ none |  |
| `pkg/gen/file_106.go` | ⚠️ none |  |
| `pkg/gen/file_107.go` | ⚠️ none |  |
| `pkg/gen/file_108.go` | ⚠️ none |  |
| `pkg/gen/file_109.go` | ⚠️ none |  |
| `pkg/gen/file_110.go` | ⚠️ none |  |
| `pkg/gen/file_111.go` | ⚠️ none |  |
| `pkg/gen/file_112.go` | ⚠️ none |  |
| `pkg/gen/file_113.go` | ⚠️ none |  |
| `pkg/gen/file_114.go` | ⚠️ none |  |
| `pkg/gen/file_115.go` | ⚠️ none |  |
| `pkg/gen/file_116.go` | ⚠️ none |  |
| `pkg/gen/file_117.go` | ⚠️ none |  |
| `pkg/gen/file_118.go` | ⚠️ none |  |
| `pkg/gen/file_119.go` | ⚠️ none |  |
| `pkg/gen/file_120.go` | ⚠️ none |  |
| `pkg/gen/file_121.go` | ⚠️ none |  |
| `pkg/gen/file_122.go` | ⚠️ none |  |
| `pkg/gen/file_123.go` | ⚠️ none |  |
| `pkg/gen/file_124.go` | ⚠️ none |  |
| `pkg/gen/file_125.go` | ⚠️ none |  |
| `pkg/gen/file_126.go` | ⚠️ none |  |
| `pkg/gen/file_127.go` | ⚠️ none |  |
| `pkg/gen/file_128.go` | ⚠️ none |  |
| `pkg/gen/file_129.go` | ⚠️ none |  |
| `pkg/gen/file_130.go` | ⚠️ none |  |
| `pkg/gen/file_131.go` | ⚠️ none |  |
| `pkg/gen/file_132.go` | ⚠️ none |  |
| `pkg/gen/file_133.go` | ⚠️ none |  |
| `pkg/gen/file_134.go` | ⚠️ none |  |
| `pkg/gen/file_135.go` | ⚠️ none |  |
| `pkg/gen/file_136.go` | ⚠️ none |  |
| `pkg/gen/file_137.go` | ⚠️ none |  |
| `pkg/gen/file_138.go` | ⚠️ none |  |
| `pkg/gen/file_139.go` | ⚠️ none |  |
| `pkg/gen/file_140.go` | ⚠️ none |  |
| `pkg/gen/file_141.go` | ⚠️ none |  |
| `pkg/gen/file_142.go` | ⚠️ none |  |
| `pkg/gen/file_143.go` | ⚠️ none |  |
| `pkg/gen/file_144.go` | ⚠️ none |  |
| `pkg/gen/file_145.go` | ⚠️ none |  |
| `pkg/gen/file_146.go` | ⚠️ none |  |
| `pkg/gen/file_147.go` | ⚠️ none |  |
| `pkg/gen/file_148.go` | ⚠️ none |  |
| `pkg/gen/file_149.go` | ⚠️ none |  |
| `pkg/gen/file_150.go` | ⚠️ none |  |
| `pkg/gen/file_151.go` | ⚠️ none |  |
| `pkg/gen/file_152.go` | ⚠️ none |  |
| `pkg/gen/file_153.go` | ⚠️ none |  |
| `pkg/gen/file_154.go` | ⚠️ none |  |
| `pkg/gen/file_155.go` | ⚠️ none |  |
| `pkg/gen/file_156.go` | ⚠️ none |  |
| `pkg/gen/file_157.go` | ⚠️ none |  |
| `pkg/gen/file_158.go` | ⚠️ none |  |
| `pkg/gen/file_159.go` | ⚠️ none |  |
| `pkg/gen/file_160.go` | ⚠️ none |  |
| `pkg/gen/file_161.go` | ⚠️ none |  |
| `pkg/gen/file_162.go` | ⚠️ none |  |
| `pkg/gen/file_163.go` | ⚠️ none |  |
| `pkg/gen/file_164.go` | ⚠️ none |  |
| `pkg/gen/file_165.go` | ⚠️ none |  |
| `pkg/gen/file_166.go` | ⚠️ none |  |
| `pkg/gen/file_167.go` | ⚠️ none |  |
| `pkg/gen/file_168.go` | ⚠️ none |  |
| `pkg/gen/file_169.go` | ⚠️ none |  |
| `pkg/gen/file_170.go` | ⚠️ none |  |
| `pkg/gen/file_171.go` | ⚠️ none |  |
| `pkg/gen/file_172.go` | ⚠️ none |  |
| `pkg/gen/file_173.go` | ⚠️ none |  |
| `pkg/gen/file_174.go` | ⚠️ none |  |
| `pkg/gen/file_175.go` | ⚠️ none |  |
| `pkg/gen/file_176.go` | ⚠️ none |  |
| `pkg/gen/file_177.go` | ⚠️ none |  |
| `pkg/gen/file_178.go` | ⚠️ none |  |
| `pkg/gen/file_179.go` | ⚠️ none |  |
| `pkg/gen/file_180.go` | ⚠️ none |  |
| `pkg/gen/file_181.go` | ⚠️ none |  |
| `pkg/gen/file_182.go` | ⚠️ none |  |
| `pkg/gen/file_183.go` | ⚠️ none |  |
| `pkg/gen/file_184.go` | ⚠️ none |  |
| `pkg/gen/file_185.go` | ⚠️ none |  |
| `pkg/gen/file_186.go` | ⚠️ none |  |
| `pkg/gen/file_187.go` | ⚠️ none |  |
| `pkg/gen/file_188.go` | ⚠️ none |  |
| `pkg/gen/file_189.go` | ⚠️ none |  |
| `pkg/gen/file_190.go` | ⚠️ none |  |
| `pkg/gen/file_191.go` | ⚠️ none |  |
| `pkg/gen/file_192.go` | ⚠️ none |  |
| `pkg/gen/file_193.go` | ⚠️ none |  |
| `pkg/gen/file_194.go` | ⚠️ none |  |
| `pkg/gen/file_195.go` | ⚠️ none |  |
| `pkg/gen/file_196.go` | ⚠️ none |  |
| `pkg/gen/file_197.go` | ⚠️ none |  |
| `pkg/gen/file_198.go` | ⚠️ none |  |
| `pkg/gen/file_199.go` | ⚠️ none |  |
| `.github/workflows/deploy.yml` | ⚠️ none | **sensitive path: CI config** |

</details>

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| `gitleaks detect --no-banner` | ✅ passed | 14:31 |
| `go test ./internal/auth/...` | ✅ passed | 14:33 |

*Recorded from the agent session — advisory only. CI remains authoritative.*

### Coverage gaps
⚠️ 201 changed files have no session evidence. They may be hand-edits or
changes from an uninstrumented tool.
