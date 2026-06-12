# gocan 开发常用命令。
# 安装 just: https://just.systems  (`brew install just` / `cargo install just` / `apt install just`)
# 列出所有任务: `just --list`

# 默认任务：跑 race 测试
default: test

# ---------- 测试与质量 ----------

# 单测 + race
test:
    go test -race ./...

# 覆盖率：生成 coverage.out 并打开 HTML 报告
cover:
    go test -race -covermode=atomic -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# 静态检查（Linux + Windows 视角各跑一遍，覆盖 build tag 文件）
lint:
    @echo "==> gofmt"
    @out=$(gofmt -l .); if [ -n "$out" ]; then echo "$out"; exit 1; fi
    @echo "==> go vet (host)"
    go vet ./...
    @echo "==> go vet (GOOS=windows)"
    GOOS=windows go vet ./...
    @echo "==> golangci-lint (host)"
    golangci-lint run ./...
    @echo "==> golangci-lint (GOOS=windows)"
    GOOS=windows golangci-lint run ./...

# 跨平台编译验证（windows 是主目标）
build-cross:
    GOOS=windows GOARCH=amd64 go build ./...
    GOOS=windows GOARCH=386   go build ./...
    GOOS=linux   GOARCH=amd64 go build ./...
    GOOS=darwin  GOARCH=arm64 go build ./...

# 本地完整 CI：push 前先跑这个，全绿才推
ci: lint test build-cross
    @echo "==> go mod tidy"
    @cp go.mod .go.mod.before
    @if [ -f go.sum ]; then cp go.sum .go.sum.before; else touch .go.sum.before; fi
    @go mod tidy
    @cmp -s go.mod .go.mod.before || (echo "go.mod is not tidy"; rm -f .go.mod.before .go.sum.before; exit 1)
    @if [ -f go.sum ]; then cmp -s go.sum .go.sum.before || (echo "go.sum is not tidy"; rm -f .go.mod.before .go.sum.before; exit 1); fi
    @rm -f .go.mod.before .go.sum.before
    @echo "✅ 本地 CI 通过，可以 push"

# 清理本地产物
clean:
    rm -f coverage.out
    go clean -cache -testcache

# ---------- 分支管理 ----------
# 工作流：
#   1. just new feat/xxx      —— 从最新 main 开新分支
#   2. 在分支上写代码 + commit
#   3. just ci                —— 本地全检查
#   4. just push              —— 推到 origin
#   5. 在 GitHub 上发 PR / 等合并
#   6. just finish feat/xxx   —— 切回 main / 拉新代码 / 删本地分支

# 从最新 main 开一个新分支
# 用法: just new feat/event-mode-windows
new branch:
    @if [ -z "{{branch}}" ]; then echo "用法: just new <branch-name>"; exit 1; fi
    @if [ "{{branch}}" = "main" ]; then echo "❌ 不能用 main 作分支名"; exit 1; fi
    git fetch origin
    git checkout main
    git pull --ff-only origin main
    git checkout -b {{branch}}
    @echo "✅ 已切到新分支 {{branch}}（基于最新 main）"

# 推送当前分支到 origin（首次 push 自动 -u）
push:
    @branch=$(git rev-parse --abbrev-ref HEAD); \
        if [ "$branch" = "main" ]; then \
            echo "❌ 拒绝直接 push main —— 请走分支 + PR"; exit 1; \
        fi; \
        git push -u origin "$branch"

# 收尾：远端 PR 已合并后调用，切回 main / 拉最新 / 删本地分支
# 用法: just finish feat/event-mode-windows
finish branch:
    @if [ -z "{{branch}}" ]; then echo "用法: just finish <branch-name>"; exit 1; fi
    @if [ "{{branch}}" = "main" ]; then echo "❌ 拒绝 finish main"; exit 1; fi
    git checkout main
    git pull --ff-only origin main
    @# -d 会拒绝未合并的分支；用户可改 -D 强删（destructive）
    git branch -d {{branch}}
    @echo "✅ 已删除本地分支 {{branch}}（远端 origin/{{branch}} 由 GitHub PR 合并时自动清理）"

# 查看本分支未推送的 commit
unpushed:
    @branch=$(git rev-parse --abbrev-ref HEAD); \
        echo "未推送的 commit (origin/$branch..HEAD):"; \
        git log --oneline "origin/$branch..HEAD" 2>/dev/null || echo "(分支未在远端，全部 commit 都未推送)"

# 显示当前仓库状态摘要
status:
    @echo "==> 分支"
    @git rev-parse --abbrev-ref HEAD
    @echo "==> 状态"
    @git status --short
    @echo "==> 最近 5 个 commit"
    @git log --oneline -5

# 创建本地开发 / 集成测试用的虚拟 CAN 接口
vcan-up:
    sudo scripts/setup-vcan.sh up vcan0 vcan1

# 删除虚拟 CAN 接口
vcan-down:
    sudo scripts/setup-vcan.sh down vcan0 vcan1
