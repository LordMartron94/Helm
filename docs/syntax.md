# Syntax

This document serves as the official documentation for the Helm syntax.

## Playground / Example

Below I will fiddle around with my intended syntax.

```

BUILD_DIR = "build"
CACHE_DIR = "${BUILD_DIR}/cache"
TEST_DIR  = "libs/test"

target test(NAME, AGE?) {
    help = "Prints your name and optionally your age."

	cache {
	    inputs = glob(TEST_DIR, exclude="*.go")
	    output = path(CACHE_DIR, "testing")
	    bypass = true // if user runs the target, can bypass cache-check with --bypass-cache
	}

	// No imperative logic (if-branching)...
	// We do allow conditional execution using simple when statements.

	run "echo 'hello ${NAME}!'" {
	    when = not_defined(AGE)
	}

	run "echo 'hello ${NAME} of ${AGE} years!'" {
	    when = defined(AGE)
	}

    run "echo 'finished task'"
    run_script "./id_gen.sh"

    depends_on [ 
    	optional "pre-generate" 
	]
    
    parallel { 
        exec "build-stepA"
        exec "build-stepB"
    }
    
    exec "run"
    exec "core-tests"
    exec confirm "optional-tests"
    exec confirm "benchmark"
}

```