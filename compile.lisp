#!/usr/bin/sbcl --script
(load "build/quicklisp/setup.lisp")
(asdf:load-system :shout)
(sb-ext:save-lisp-and-die
  "shout"
  :compression nil
  :executable  t
  :toplevel #'shout:shout)
