package vmm

import "testing"

func TestIsManagedRuntimeIncludesElpian(t *testing.T) {
	supported := []string{"wasm", "javascript", "elpify", "elpian", "fire", " ELPian "}
	for _, runtime := range supported {
		if !isManagedRuntime(runtime) {
			t.Fatalf("runtime %q should be managed", runtime)
		}
	}

	notSupported := []string{"", "docker", "python"}
	for _, runtime := range notSupported {
		if isManagedRuntime(runtime) {
			t.Fatalf("runtime %q should not be managed", runtime)
		}
	}
}
