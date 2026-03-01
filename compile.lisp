#!/usr/bin/sbcl --script
(load "build/quicklisp/setup.lisp")
(push (truename ".") asdf:*central-registry*)
(asdf:load-system :shout)
(sb-ext:save-lisp-and-die
  "shout"
  :compression t
  :executable  t
  :toplevel #'shout:shout)
