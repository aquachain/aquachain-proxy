aquachain-proxy: *.go */*.go www/* www/*/*
	go build -o $@
aquachain-proxy-riscv: *.go */*.go www/* www/*/*
	# riscv
	GOARCH=riscv64 go build -o $@
aquachain-proxy-arm: *.go */*.go www/* www/*/*
	# aka aarch64
	GOARCH=arm64 go build -o $@
all: aquachain-proxy aquachain-proxy-riscv aquachain-proxy-arm
clean:
	@rm -vf aquachain-proxy-* aquachain-proxy
dist-clean: clean
	@rm -vf aquachain-proxy-* aquachain-proxy
	@rm -vi aquaproxy.json || true