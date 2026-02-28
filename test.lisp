#!/usr/bin/env sbcl --script
(load "build/quicklisp/setup.lisp")
(push (truename ".") asdf:*central-registry*)
(require "prove")
(prove:run :shout-test :reporter :list)
