(in-package :shout-test)

(plan nil)

(subtest "shout-log formatting"
  (let ((output (with-output-to-string (*error-output*)
                  (api:shout-log "test" "hello ~A" "world"))))
    (ok (cl-ppcre:scan
          "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2} \\[test\\] hello world\\n$"
          output)
        (format nil "shout-log output matches ISO timestamp format (got ~S)" output))))

(subtest "State expiry removes stale states"
  (let ((saved-states   api::*states*)
        (saved-expiry   api::*expiry*)
        (saved-dirty    api::*states-dirty*)
        (tmpfile        (format nil "/tmp/shout-test-expiry-~D.db" (get-universal-time))))
    (unwind-protect
      (progn
        ;; Two states: one with occurred-at far in the past, one recent
        (let ((old-event (make-instance 'api::event
                                        :ok t
                                        :message "old"
                                        :link ""
                                        :occurred-at 1))
              (new-event (make-instance 'api::event
                                        :ok t
                                        :message "new"
                                        :link ""
                                        :occurred-at (api::unix-now))))
          (setf api::*states*
                (list (cons "old-topic"
                            (make-instance 'api::state
                                           :name "old-topic"
                                           :status "working"
                                           :last-notified-at 1
                                           :first-event old-event
                                           :last-event old-event))
                      (cons "new-topic"
                            (make-instance 'api::state
                                           :name "new-topic"
                                           :status "working"
                                           :last-notified-at (api::unix-now)
                                           :first-event new-event
                                           :last-event new-event))))
        (setf api::*expiry* 60)
        (setf api::*states-dirty* nil)

        (api:scan tmpfile)

        (ok (null (assoc "old-topic" api::*states* :test #'equal))
            "Expired state should be removed")
        (ok (not (null (assoc "new-topic" api::*states* :test #'equal)))
            "Recent state should remain")
        (ok api::*states-dirty*
            "*states-dirty* should be set after expiry")))
      ;; Restore
      (setf api::*states* saved-states
            api::*expiry* saved-expiry
            api::*states-dirty* saved-dirty)
      (ignore-errors (delete-file tmpfile)))))

(subtest "Dirty flag — skip write when clean"
  (let ((saved-states api::*states*)
        (saved-expiry api::*expiry*)
        (saved-dirty  api::*states-dirty*)
        (path         (format nil "/tmp/shout-test-nope-~D.db" (get-universal-time))))
    (unwind-protect
      (progn
        (setf api::*states-dirty* nil)
        (setf api::*expiry* nil)
        (setf api::*states* '())

        ;; scan should complete without error even with nonexistent path
        ;; because it never attempts to write
        (api:scan path)
        (ok (not (probe-file path))
            "Database file should NOT be created when not dirty"))
      (setf api::*states* saved-states
            api::*expiry* saved-expiry
            api::*states-dirty* saved-dirty)
      (ignore-errors (delete-file path)))))

(subtest "Dirty flag — write when dirty"
  (let ((saved-states api::*states*)
        (saved-expiry api::*expiry*)
        (saved-dirty  api::*states-dirty*)
        (tmpfile      (format nil "/tmp/shout-test-dirty-~D.db" (get-universal-time))))
    (unwind-protect
      (progn
        (let ((ev (make-instance 'api::event
                                 :ok t
                                 :message "test"
                                 :link ""
                                 :occurred-at (api::unix-now))))
          (setf api::*states*
                (list (cons "dirty-topic"
                            (make-instance 'api::state
                                           :name "dirty-topic"
                                           :status "working"
                                           :last-notified-at (api::unix-now)
                                           :first-event ev
                                           :last-event ev)))))
        (setf api::*states-dirty* t)
        (setf api::*expiry* nil)

        (api:scan tmpfile)

        (ok (probe-file tmpfile)
            "Database file should be created when dirty")
        (ok (not api::*states-dirty*)
            "*states-dirty* should be nil after successful write"))
      (setf api::*states* saved-states
            api::*expiry* saved-expiry
            api::*states-dirty* saved-dirty)
      (ignore-errors (delete-file tmpfile)))))

(subtest "TLS config validation — cert without key errors"
  (is-error (api:run :tls-cert "/tmp/fake-cert.pem")
            'simple-error
            "tls-cert without tls-key should signal an error"))

(subtest "TLS config validation — key without cert errors"
  (is-error (api:run :tls-key "/tmp/fake-key.pem")
            'simple-error
            "tls-key without tls-cert should signal an error"))

(finalize)
