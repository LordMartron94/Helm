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

    // A flat, un-nestable when-block. 
    // The compiler must throw a fatal error if a 'when' is placed inside a 'when'.

    /*
    When cannot be nested and cannot contain complex logic.

    Else is also not possible, it would be the inverse (not_defined)

    You can also check for example:

    when equals(AGE, 12)

    or

    when not_equals(AGE, 12)

    BUT you cannot combine like:
    when defined(AGE) && equals(AGE, 12) ; this is impossible.

    If you want complex logic either:
    - Resolve in a previous target
    - Move it to a shell script
    */

	when defined(AGE) {
	    run "echo 'hello ${NAME} of ${AGE} years!'"
	    run "echo 'saving age data...'"
	}

	when not_defined(AGE) {
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