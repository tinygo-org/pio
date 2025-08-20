
clean:
	@rm -rf build

FMT_PATHS = ./

fmt-check:
	@unformatted=$$(gofmt -l $(FMT_PATHS)); [ -z "$$unformatted" ] && exit 0; echo "Unformatted:"; for fn in $$unformatted; do echo "  $$fn"; done; exit 1

smoke-test:
	@mkdir -p build
	tinygo build -target pico-w -size short -o build/test.uf2 ./rp2-pio/examples/blinky
	tinygo build -target pico-w -size short -o build/test.uf2 ./rp2-pio/examples/i2s
	tinygo build -target pico-w -size short -o build/test.uf2 ./rp2-pio/examples/pulsar
	tinygo build -target pico-w -size short -o build/test.uf2 ./rp2-pio/examples/tufty
	tinygo build -target pico-w -size short -o build/test.uf2 ./rp2-pio/examples/ws2812b
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/blinky
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/i2s
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/pulsar
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/neosimple
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/rxfifoput
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/rxfifoputget
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/tufty
	tinygo build -target pico2 -size short -o build/test.uf2 ./rp2-pio/examples/ws2812b

test: clean fmt-check smoke-test
