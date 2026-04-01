# Syntax

This document serves as the official documentation for the Helm syntax.

## Playground / Example

Below I will fiddle around with my intended syntax.

```

BUILD_DIR = "build"
CACHE_DIR = "$(BUILD_DIR)/cache"
TEST_DIR = "libs/test"

target test(NAME, AGE?) {
    help = "Prints your name and optionally your age."

    cache {
        in = "$(TEST_DIR)/** -not -name \"*.go\""
        out = "$(CACHE_DIR)/testing"
        allow_bypass = true
    }

    ifdef AGE {
        echo "hello $(NAME) of $(AGE) years!"
    } else {
        echo "hello $(NAME)!"
    }

    run "echo 'finished task'"
    run_script "./id_gen.sh"

    // ?  = continue if failed
    // ?? = require user confirm before running 

    depends_on [ "pre-generate?" ]
    
    parallel {
        exec "build-stepA"
        exec "build-stepB"
    }
    
    exec "run"
    exec "core-tests"
    exec "optional-tests??"
    exec "benchmark??"
}

```