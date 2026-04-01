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
        bypass = true
    }

    // A flat, un-nestable if-block. 
    // The compiler must throw a fatal error if an 'if' is placed inside an 'if'.
    if defined(AGE) {
        run "echo 'hello ${NAME} of ${AGE} years!'"
        run "echo 'saving age data...'"
    } else {
        run "echo 'hello ${NAME}!'"
    }

    run "echo 'finished task'"
    run_script "./id_gen.sh"

    // Standardize dependencies as simple strings. 
    // If a dependency needs configuration, it becomes an object/block.
    depends_on [ 
        "pre-generate" { optional = true } 
    ]
    
    parallel { 
        exec "build-stepA"
        exec "build-stepB"
    }
    
    exec "run"
    exec "core-tests"
    
    // Standardized trailing configuration, identical to 'run' and 'depends_on'
    exec "optional-tests" { confirm = true }
    exec "benchmark" { confirm = true }
}

```