(in-package :shout-test)

(defvar *no-params*   '())
(defvar *no-metadata* '())

(defvar *args* '())
(rules::register-plugin
  'made-it
  #'(lambda (args)
      (setf *args* args)))

(defun triggers (what &optional msg)
  (is *args* (list what)
      (or msg
          (format nil "Should have triggered '~A'" what))))

(defun parse (form)
  (cond ((symbolp form) (symbol-name form))
        ((atom form) (format nil "~S" form))
        (t (format nil "(~{~A ~})" (mapcar #'parse form)))))

(defun try (rules &key params metadata)
  (rules:eval/rules
    (rules:load/rules (parse rules))
    (or params *no-params*)
    (or metadata *no-metadata*)))

(plan nil)
(subtest "Plugin Registration"
  (ok (rules::registered-plugin? 'made-it)
      "'made-it (calling package) should be registered")
  (ok (rules::registered-plugin? :made-it)
      ":made-it (KEYWORD package) should be registered")
  (ok (not (rules::registered-plugin? 'not-a-thing))
      "'not-a-thing shoult not be registered"))

(subtest "Evaluation"
  (try `((for *
           (when *
             (made-it "here")))))
  (triggers "here" "FOR * / WHEN * always fires")


  (try `((for *
           (when *
             (made-it "no further than here"))
           (when (on weekdays)
             (made-it "to the weekday!"))
           (when (on weekends)
             (made-it "to the weekday!")))))
  (triggers "no further than here"
      "Only the first matching WHEN should fire")

  (try `((for "topic"
           (when * (made-it "to the topic")))
             (for "off-topic"
           (when * (made-it "wrong"))))
       :params (pairlis '(:topic) '("topic")))
  (triggers "to the topic"
      "FOR should match topic expicitly")

  (try `((for (or (is "topic")
                  (is "something else"))
           (when * (made-it "to the topic")))
         (for "off-topic"
           (when * (made-it "wrong"))))
       :params (pairlis '(:topic) '("topic")))
  (triggers "to the topic"
      "FOR should match topic in an expression tree")

  (try `((for *
           (when ((metadata? super-important))
             (made-it "shouldn't make it..."))
           (when ((metadata? escalate))
             (made-it (metadata escalate)))
           (when * (made-it "normal"))))
       :metadata (pairlis '(:escalate) '("escalated")))
  (triggers "escalated" "Metadata evaluates properly")

  (try `((set msg "a message")
         (for *
           (when *
             (made-it (value msg))))))
  (triggers "a message" "Variables should work")

  (try `((set hooks (map "default"  "the default"
                         "override" "something"))
         (for *
           (when *
             (made-it (lookup hooks "override" "default"))))))
  (triggers "something" "Map lookups with variable keys should work")

  (try `((set hooks (map "this" "to that"))
         (for *
           (when ((lookup hooks "x" "y" "z"))
             (made-it "to the wrong place"))
           (when *
             (made-it (lookup hooks "this"))))))
  (triggers "to that" "Map lookups work in conditionals and value positions")

  (let ((ruleset
          `((for *
              (when ((or a b))
                (made-it "a or b was true"))
              (when *
                (made-it "neither were true"))))))
    (try ruleset :params (pairlis '(:a :b) '(t t)))
    (triggers "a or b was true" "(OR t t) is true")

    (try ruleset :params (pairlis '(:a :b) '(t nil)))
    (triggers "a or b was true" "(OR t nil) is true")

    (try ruleset :params (pairlis '(:a :b) '(nil t)))
    (triggers "a or b was true" "(OR nil t) is true")

    (try ruleset :params (pairlis '(:a :b) '(nil nil)))
    (triggers "neither were true" "(OR nil nil) is false"))

  (let ((ruleset
          `((for *
              (when ((not a))
                (made-it "(NOT a) was true"))
              (when *
                (made-it "(NOT a) was false"))))))
    (try ruleset :params (pairlis '(:a) '(t)))
    (triggers "(NOT a) was false")

    (try ruleset :params (pairlis '(:a) '(nil)))
    (triggers "(NOT a) was true"))

  (let ((ruleset
          `((for *
              (when *
                (made-it (if test "consequence" "alternate")))))))
    (try ruleset :params (pairlis '(:test) '(t)))
    (triggers "consequence"
              "(IF t ...) should trigger the consequent")
    (try ruleset :params (pairlis '(:test) '(nil)))
    (triggers "alternate"
              "(IF nil ...) should trigger the alternate"))

  (let ((ruleset
          `((for *
              (when *
                (made-it (if test "consequence")))))))
    (try ruleset :params (pairlis '(:test) '(t)))
    (triggers "consequence"
              "(IF t ...) should trigger the consequent")
    (try ruleset :params (pairlis '(:test) '(nil)))
    (triggers nil "A nil alternate for (IF ...) is ok"))

  (try `((set var "variable")
         (for *
           (when *
             (made-it (concat "this is a" (value var)))))))
  (triggers
    (format nil "this is a~%variable~%")
    "Concat combines strings mapped with trailing newlines")

  (try `((for *
           (when *
             (made-it message))))
       :params (pairlis '(:message) '("a message")))
  (triggers "a message"
    "Symbols evaluate to the named parameter")

  (try `((for *
           (when *
             (made-it nil)))))
  (triggers nil "NIL evaluates to nil")

  (try `((for *
           (when *
             (made-it "this is a $message"))))
       :params (pairlis '(:message) '("parameter")))
  (triggers "this is a parameter"
    "String interpolation of parameters works")

  (try `((for *
           (when *
             (made-it "this is $[meta]"))))
       :metadata (pairlis '(:meta) '("metadata")))
  (triggers "this is metadata"
    "String interpolation of metadata works"))

(subtest "Time-based predicates"
  (let ((rules::*NOW* (encode-universal-time 0 14 2 29 8 1997)))
    ; It's August 29th, 2:14am local time (a Friday)
    ; SkyNet has become self-aware.
    ; How do we notify about that?

    (try `((for *
             (when ((on weekends))
               (made-it "not yet..."))
             (when ((on weekdays))
               (made-it "skynet has become self-aware"))
             (when * (made-it "wrong")))))
    (triggers "skynet has become self-aware")

    (try `((for *
             (when ((on friday))
               (made-it "skynet has become self-aware"))
             (when * (made-it "wrong")))))
    (triggers "skynet has become self-aware")

    (try `((for *
             (when ((after 0800 am))
               (made-it "too late"))
             (when * (made-it "correct")))))
    (triggers "correct")

    (try `((for *
             (when ((before 0800 am))
               (made-it "on time"))
             (when * (made-it "wrong")))))
    (triggers "on time")

    (try `((for *
             (when ((after 0100 pm))
               (made-it "too late"))
             (when * (made-it "on time")))))
    (triggers "on time")

    (try `((for *
             (when ((from 0100 am to 0300 am))
               (made-it "on time"))
             (when * (made-it "wrong")))))
    (triggers "on time")

    (ok t))
  (ok t))

(subtest "Syntax errors"
  (defun is-syntax-error (form &key params metadata)
    (is-error (try form :params params :metadata metadata)
              'simple-error))

  (is-syntax-error `((set 42)))
  (is-syntax-error `((set var)))
  (is-syntax-error `((foo bar)))
  (is-syntax-error `((for * (foo bar))))
  (is-syntax-error `((for * (when * bar))))
  (is-syntax-error `((for * (when * (foo bar)))))
  (is-syntax-error `((for * (when ((foo bar)) (made-it "fail")))))
  (is-syntax-error `((for * (when ((after 0800 bc)) (made-it "fail")))))
  (ok t))

(subtest "Notification Reminders"
  (is (cons :remind (* 86400))
      (try
        `((for *
            (when *
              (remind 24 hours)
              (made-it "notified")))))
      "Evaluating the ruleset should return the 24h reminder")
  (triggers "notified"))

(subtest "AND logic"
  (let ((ruleset
          `((for *
              (when ((and a b))
                (made-it "a and b was true"))
              (when *
                (made-it "not both true"))))))
    (try ruleset :params (pairlis '(:a :b) '(t t)))
    (triggers "a and b was true" "(AND t t) is true")
    (try ruleset :params (pairlis '(:a :b) '(t nil)))
    (triggers "not both true" "(AND t nil) is false")
    (try ruleset :params (pairlis '(:a :b) '(nil t)))
    (triggers "not both true" "(AND nil t) is false")
    (try ruleset :params (pairlis '(:a :b) '(nil nil)))
    (triggers "not both true" "(AND nil nil) is false")))

(subtest "Nested logic"
  ;; (and (or a b) (not c)) — true when at least one of a/b and c is false
  (let ((ruleset
          `((for *
              (when ((and (or a b) (not c)))
                (made-it "matched"))
              (when *
                (made-it "no match"))))))
    (try ruleset :params (pairlis '(:a :b :c) '(t nil nil)))
    (triggers "matched" "(AND (OR t nil) (NOT nil)) is true")
    (try ruleset :params (pairlis '(:a :b :c) '(nil nil nil)))
    (triggers "no match" "(AND (OR nil nil) (NOT nil)) is false")
    (try ruleset :params (pairlis '(:a :b :c) '(t t t)))
    (triggers "no match" "(AND (OR t t) (NOT t)) is false")))

(subtest "Time unit aliases"
  ;; Days
  (is (cons :remind 86400) (try `((for * (when * (remind 1 d)))))
      "1 d = 86400s")
  (is (cons :remind 86400) (try `((for * (when * (remind 1 day)))))
      "1 day = 86400s")
  (is (cons :remind 172800) (try `((for * (when * (remind 2 days)))))
      "2 days = 172800s")
  ;; Hours
  (is (cons :remind 3600) (try `((for * (when * (remind 1 h)))))
      "1 h = 3600s")
  (is (cons :remind 3600) (try `((for * (when * (remind 1 hour)))))
      "1 hour = 3600s")
  ;; Minutes
  (is (cons :remind 60) (try `((for * (when * (remind 1 m)))))
      "1 m = 60s")
  (is (cons :remind 60) (try `((for * (when * (remind 1 min)))))
      "1 min = 60s")
  (is (cons :remind 60) (try `((for * (when * (remind 1 minute)))))
      "1 minute = 60s")
  (is (cons :remind 120) (try `((for * (when * (remind 2 minutes)))))
      "2 minutes = 120s")
  ;; Seconds
  (is (cons :remind 30) (try `((for * (when * (remind 30 s)))))
      "30 s = 30s")
  (is (cons :remind 30) (try `((for * (when * (remind 30 sec)))))
      "30 sec = 30s")
  (is (cons :remind 30) (try `((for * (when * (remind 30 second)))))
      "30 second = 30s")
  (is (cons :remind 30) (try `((for * (when * (remind 30 seconds)))))
      "30 seconds = 30s"))

(subtest "Metadata edge cases"
  ;; (metadata? key) returns nil when key doesn't exist
  (try `((for *
           (when ((metadata? missing))
             (made-it "found"))
           (when *
             (made-it "not found")))))
  (triggers "not found" "(metadata? missing-key) is false")

  ;; (metadata key) returns "" when key doesn't exist
  (try `((for *
           (when *
             (made-it (metadata missing))))))
  (triggers "" "(metadata missing-key) returns empty string"))

(subtest "Weekend matching"
  ;; Saturday, Jan 4, 1997 at noon
  (let ((rules::*NOW* (encode-universal-time 0 0 12 4 1 1997)))
    (try `((for *
             (when ((on weekends))
               (made-it "weekend"))
             (when *
               (made-it "weekday")))))
    (triggers "weekend" "Saturday matches (on weekends)")

    (try `((for *
             (when ((on saturday))
               (made-it "saturday"))
             (when *
               (made-it "wrong")))))
    (triggers "saturday" "Saturday matches (on saturday)")))

(subtest "Multiple handlers in body"
  ;; Two plugin calls in one WHEN body — both should fire
  (try `((for *
           (when *
             (made-it "first")
             (made-it "second")))))
  ;; Last call wins for *args* — verifies both calls executed
  (triggers "second" "Last handler in body sets final result"))

(subtest "Map lookup miss"
  ;; lookup returns nil when no keys match
  (try `((set m (map "a" "found"))
         (for *
           (when *
             (made-it (lookup m "x" "y" "z"))))))
  (triggers nil "Lookup returns nil when no keys match"))

(subtest "FOR topic non-match skips"
  (try `((for "other-topic"
           (when * (made-it "wrong")))
         (for *
           (when * (made-it "fallback"))))
       :params (pairlis '(:topic) '("my-topic")))
  (triggers "fallback" "Non-matching FOR is skipped, fallback FOR * matches"))

(subtest "Parse error in load/rules"
  (is-error (rules:load/rules "((for * (when * (bad-syntax")
            'simple-error
            "Unterminated sexp raises error")
  (is-error (rules:load/rules "#<invalid>")
            'simple-error
            "Invalid read syntax raises error"))

(subtest "String interpolation of all parameters"
  (try `((for *
           (when *
             (made-it "$topic: $status - $message ($link)"))))
       :params (pairlis '(:topic :status :message :link)
                         '("ci/pipeline" "now broken" "build failed" "http://ci/1")))
  (triggers "ci/pipeline: now broken - build failed (http://ci/1)"
            "All four parameter placeholders interpolate"))

(subtest "Sunday matches weekends"
  ;; Sunday, Jan 5, 1997 at noon
  (let ((rules::*NOW* (encode-universal-time 0 0 12 5 1 1997)))
    (try `((for *
             (when ((on weekends))
               (made-it "weekend"))
             (when *
               (made-it "weekday")))))
    (triggers "weekend" "Sunday matches (on weekends)")

    (try `((for *
             (when ((on sunday))
               (made-it "sunday"))
             (when *
               (made-it "wrong")))))
    (triggers "sunday" "Sunday matches (on sunday)")))

(subtest "Multiple days in ON"
  ;; Friday, Aug 29, 1997
  (let ((rules::*NOW* (encode-universal-time 0 14 2 29 8 1997)))
    (try `((for *
             (when ((on monday wednesday friday))
               (made-it "matched"))
             (when *
               (made-it "no match")))))
    (triggers "matched" "Friday matches (on monday wednesday friday)")

    (try `((for *
             (when ((on monday wednesday))
               (made-it "matched"))
             (when *
               (made-it "no match")))))
    (triggers "no match" "Friday doesn't match (on monday wednesday)")))

(subtest "Hours plural alias"
  (is (cons :remind 7200) (try `((for * (when * (remind 2 hours)))))
      "2 hours = 7200s"))

(subtest "IS in WHEN condition"
  (try `((for *
           (when ((is "my-topic"))
             (made-it "matched"))
           (when *
             (made-it "no match"))))
       :params (pairlis '(:topic) '("my-topic")))
  (triggers "matched" "(is \"my-topic\") matches topic in WHEN condition")

  (try `((for *
           (when ((is "other"))
             (made-it "matched"))
           (when *
             (made-it "no match"))))
       :params (pairlis '(:topic) '("my-topic")))
  (triggers "no match" "(is \"other\") doesn't match topic in WHEN condition"))

(subtest "FROM/TO boundary values"
  ;; Exactly at start time: 8:00:00 AM, Jan 1, 1997
  (let ((rules::*NOW* (encode-universal-time 0 0 8 1 1 1997)))
    (try `((for *
             (when ((from 0800 am to 0500 pm))
               (made-it "in range"))
             (when *
               (made-it "out of range")))))
    (triggers "in range" "Exact start time matches (>= boundary)"))

  ;; Exactly at end time: 5:00:00 PM, Jan 1, 1997
  (let ((rules::*NOW* (encode-universal-time 0 0 17 1 1 1997)))
    (try `((for *
             (when ((from 0800 am to 0500 pm))
               (made-it "in range"))
             (when *
               (made-it "out of range")))))
    (triggers "in range" "Exact end time matches (<= boundary)")))

(subtest "All matching FOR blocks fire"
  ;; Both FOR * blocks match — both execute, last one wins
  (try `((for *
           (when * (made-it "first")))
         (for *
           (when * (made-it "second")))))
  (triggers "second" "All matching FOR blocks fire, not just the first"))

(subtest "Multiple SET variables"
  (try `((set x "hello")
         (set y "world")
         (for *
           (when *
             (made-it (concat (value x) (value y)))))))
  (triggers (format nil "hello~%world~%")
            "Multiple SET variables coexist and resolve independently"))

(subtest "Concat strips nil elements"
  ;; 'missing' symbol evaluates to nil (not in params), removed by concat
  (try `((for *
           (when *
             (made-it (concat "hello" missing "world"))))))
  (triggers (format nil "hello~%world~%")
            "Concat strips nil values from output"))

(subtest "Missing metadata interpolation left as-is"
  ;; $[nonexistent] stays literal when metadata key doesn't exist
  (try `((for *
           (when *
             (made-it "value is $[nonexistent]")))))
  (triggers "value is $[nonexistent]"
            "$[key] left as-is when metadata key doesn't exist"))

(subtest "Missing parameter interpolation crashes"
  ;; BUG: $topic in string without :topic param crashes in replace-all
  ;; because (param :topic) returns nil and write-string expects a string
  (is-error (try `((for *
                     (when *
                       (made-it "$topic")))))
            'type-error
            "$param placeholder without corresponding param raises type-error"))

(subtest "Invalid time unit in remind"
  ;; ecase in time-lapse rejects unknown units
  (is-error (try `((for * (when * (remind 1 weeks)))))
            'type-error
            "Invalid time unit raises type-error from ecase"))

(subtest "FOR with no WHEN clauses"
  ;; Empty FOR body is a no-op
  (is nil (try `((for *)))
      "FOR with no WHEN clauses is a no-op returning nil"))

(finalize)
