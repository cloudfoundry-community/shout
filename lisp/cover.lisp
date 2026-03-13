#!/usr/bin/env sbcl --script
(load "build/quicklisp/setup.lisp")

;;; Enable generation of code coverage instrumentation.
(require :sb-cover)
(declaim (optimize sb-cover:store-coverage-data))

;;; Reload shout-test, ensuring that it's recompiled with
;;; the new optimization policy.
(asdf:oos 'asdf:load-op :shout-test :force t)
(asdf:oos 'asdf:load-op :shout :force t)

;;; Run the test suite.
(prove:run :shout-test :reporter :list)
(sb-cover:report "coverage/")

;;; Compute overall expression coverage percentage.
;;; sb-cover tracks per-source-path counts of (covered . total) expressions.
(let ((total-exprs 0)
      (covered-exprs 0))
  (maphash (lambda (key data)
             (declare (ignore key))
             (loop for datum across data
                   when (and datum (not (eq datum 'sb-cover::unknown)))
                   do (incf total-exprs)
                      (when (plusp (car datum))
                        (incf covered-exprs))))
           (sb-cover::code-coverage-hashtable))
  (let ((pct (if (zerop total-exprs) 0.0
                 (* 100.0 (/ covered-exprs total-exprs)))))
    (format t "~%Total expression coverage: ~,1F% (~D/~D)~%"
            pct covered-exprs total-exprs)
    ;; Write percentage to a file for Makefile to read.
    (with-open-file (out "coverage/percent.txt"
                         :direction :output
                         :if-exists :supersede)
      (format out "~,1F~%" pct))))

(declaim (optimize (sb-cover:store-coverage-data 0)))
