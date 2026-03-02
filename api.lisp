(in-package :api)

(defvar *default-port* 7109)
(defvar *default-dbfile* #p"/var/shout.db")
(defvar *default-rules* #p"/var/shout.rules")
(defvar *default-expiry* 86400)
(defvar *default-auth* '("shout" . "shout"))

;; the base offset of UNIX Epoch time into LISP Universal Time
(defvar *EPOCH* (encode-universal-time 0 0 0 1 1 1970 0))

(defun unix-now ()
  (- (get-universal-time) *EPOCH*))

(defun shout-log (prefix fmt &rest args)
  (multiple-value-bind (s m h d mo y) (decode-universal-time (get-universal-time))
    (format *error-output* "~4D-~2,'0D-~2,'0DT~2,'0D:~2,'0D:~2,'0D [~A] ~?~%"
            y mo d h m s prefix fmt args)))

(defvar *states* '())
(defvar *states-lock* (make-lock "states"))
(defvar *rules* '())
(defvar *rules-src* "")
(defvar *rules-lock* (make-lock "rules"))
(defvar *expiry* nil)
(defvar *dbfile* nil)
(defvar *shutdown* nil)
(defvar *states-dirty* nil)

(defclass event ()
  ((message
    :initarg :message
    :accessor event-message)
   (link
    :initarg :link
    :accessor event-link)
   (ok
    :initarg :ok
    :accessor event-ok?)
   (metadata
    :initarg :metadata
    :initform ()
    :accessor event-metadata)
   (occurred-at
    :initarg :occurred-at
    :initform (unix-now)
    :accessor event-occurred-at)
   (reported-at
    :initarg :reported-at
    :initform (unix-now)
    :accessor event-reported-at)))

(defclass state ()
  ((topic
    :initarg :name
    :accessor topic)
   (status
    :initarg :status
    :accessor status-of)
   (last-notified-at
    :initarg :last-notified-at
    :initform nil
    :accessor last-notified-at)
   (remind-every
    :initarg :remind-every
    :initform nil
    :accessor remind-every)
   (previous-event
    :initarg :previous-event
    :initform nil
    :accessor previous-event)
   (first-event
    :initarg :first-event
    :initform nil
    :accessor first-event)
   (last-event
    :initarg :last-event
    :initform nil
    :accessor last-event)))

(defun state-is-ok? (st)
  (and st
       (last-event st)
       (event-ok? (last-event st))))

(defun state-needs-reminder? (st)
  (and (not (state-is-ok? st))
       (remind-every st)
       (< (+ (last-notified-at st) (remind-every st))
          (unix-now))))

(defun notify-about-state (state event mode edge)
  (handler-case
    (let ((result (with-lock-held (*rules-lock*)
                    (rules:eval/rules *rules*
                      (pairlis
                        '(:topic :ok? :status :last-notified :message :link)
                        (list
                          (topic state)
                          (event-ok? event)
                          (format nil "~A ~A" mode edge)
                          (last-notified-at state)
                          (event-message event)
                          (if (equal (event-link event) "")
                            nil
                            (event-link event))))
                      (event-metadata event)))))
      (setf (last-notified-at state) (unix-now))
      (setf (remind-every state)
            (if (and result (eq (car result) :remind))
              (cdr result))))
    (error (e)
      (shout-log "notify" "error notifying about ~A: ~A" (topic state) e))))

(defun notify-announcement (topic event)
  (with-lock-held (*rules-lock*)
    (rules:eval/rules *rules*
      (pairlis
        '(:announcement? :topic :ok? :status :message :link)
        (list t topic t "worth looking into..."
              (event-message event)
              (if (equal (event-link event) "")
                  nil
                  (event-link event))))
      (event-metadata event))))

(defun trigger-edge (state event type)
  (notify-about-state state event "now" type))

(defun transition-state (e1 e2)
  (cond ((not (event-ok? e2))
         "broken")
        ((and (not (event-ok? e1))
              (event-ok? e2))
         "fixed")
        (t "working")))

(defun ingest-event (state event)
  (let ((edge (transition-state
                (last-event state)
                event)))
    (when (not (eq (event-ok? (last-event state))
                   (event-ok? event)))
      (trigger-edge state event edge)
      (setf (previous-event state)
            (last-event     state)))

    (setf (status-of state) edge
          (last-event state) event)
    state))

(defun find-state (topic)
  (cdr (assoc topic *states* :test #'equal)))

(defun add-state (topic event)
  (let ((state (make-instance 'state :name topic
                                     :status (if (event-ok? event) "working" "broken"))))
    (setf (last-notified-at state) (unix-now)
          (previous-event   state) nil
          (first-event      state) event
          (last-event       state) event
          *states* (acons topic state *states*))
    state))

(defun set-state (topic event)
  (let ((state (find-state topic)))
    (setf *states-dirty* t)
    (if state
      (ingest-event state event)
      (add-state topic event))))

(defmacro handle (url &body body)
  (let ((fn (gensym "fn")))
    `(progn
       (defun ,fn ()
         ,@body)
       (push (create-prefix-dispatcher ,url ',fn) *dispatch-table*))))

(defmacro handle-json (url &body body)
  `(handle ,url
           (setf (content-type* *reply*) "application/json")
           (format nil "~A~%" (json:encode-json-to-string
                                (progn ,@body)))))

(defun constant-time-equal (a b)
  "Constant-time string comparison to prevent timing attacks."
  (and (= (length a) (length b))
       (zerop (reduce #'logior
                (map 'list (lambda (x y) (logxor (char-code x) (char-code y))) a b)
                :initial-value 0))))

(defmacro with-auth (auth &body body)
  (let ((got-user (gensym))
        (got-pass (gensym)))
  `(multiple-value-bind (,got-user ,got-pass)
    (hunchentoot:authorization)
    (cond ((or (null ,got-user) (null ,got-pass))
           (hunchentoot:require-authorization))
          ((or (not (constant-time-equal ,got-user (car ,auth)))
               (not (constant-time-equal ,got-pass (cdr ,auth))))
           (setf (return-code *reply*) 403)
           (hunchentoot:abort-request-handler))
          (t ,@body)))))

(defvar *max-body-size* (* 1 1024 1024)) ;; 1MB

(defun json-body ()
  (let ((content-length (header-in* :content-length *request*)))
    (when (and content-length
               (> (parse-integer content-length :junk-allowed t) *max-body-size*))
      (setf (return-code* *reply*) 413)
      (hunchentoot:abort-request-handler)))
  (decode-json-from-string
    (raw-post-data :force-text t)))

(defun jref (o field)
  (cdr (assoc field o)))

(defun event-json (e)
  (if (null e)
    nil
    `((occurred_at . ,(event-occurred-at e))
      (reported_at . ,(event-reported-at e))
      (ok          . ,(event-ok? e))
      (metadata    . ,(event-metadata e))
      (message     . ,(event-message e))
      (link        . ,(event-link e)))))

(defun state-json (st)
  `((name     . ,(topic st))
    (state    . ,(status-of st))
    (notified . ,(last-notified-at st))
    (reminder . ,(remind-every st))
    (previous . ,(event-json (previous-event st)))
    (first    . ,(event-json (first-event st)))
    (last     . ,(event-json (last-event st)))))

(defun event-from-json (json)
  (when json
    (make-instance 'event
                   :message     (jref json :message)
                   :ok          (jref json :ok)
                   :metadata    (jref json :metadata)
                   :link        (jref json :link)
                   :occurred-at (jref json :occurred-at)
                   :reported-at (jref json :reported-at))))

(defun state-from-json (json)
  (make-instance 'state
                 :name             (jref json :name)
                 :status           (jref json :state)
                 :last-notified-at (jref json :notified)
                 :remind-every     (jref json :reminder)
                 :previous-event   (event-from-json (jref json :previous))
                 :first-event      (event-from-json (jref json :first))
                 :last-event       (event-from-json (jref json :last))))

(defun read-database (path)
  (let ((raw (with-open-file (in path :direction :input :if-does-not-exist nil)
               (when in
                 (json:decode-json-from-string
                   (format nil "~{~A~}"
                     (loop for line = (read-line in nil)
                           while line
                           collect line))))))
        (db '()))
    (loop for state in raw do
          (setf db (acons (jref state :name)
                          (state-from-json state)
                          db)))
    db))

(defun write-database (path db)
  (let ((tmp (make-pathname :type "tmp" :defaults path)))
    (unwind-protect
      (progn
        (with-open-file (out tmp :direction :output :if-exists :supersede)
          (format out "~A~%" (json:encode-json-to-string
                               (mapcar #'(lambda (pair)
                                           (state-json (cdr pair))) db))))
        (rename-file tmp path)
        (sb-posix:chmod (namestring (truename path)) #o600))
      (when (probe-file tmp)
        (ignore-errors (delete-file tmp))))))

(defun handle-sigterm (sig code scp)
  (declare (ignore sig code scp))
  (let ((t0 (get-internal-real-time)))
    (shout-log "shutdown" "received SIGTERM, flushing database")
    (handler-case
      (with-lock-held (*states-lock*)
        (when *dbfile*
          (write-database *dbfile* *states*)))
      (error (e)
        (shout-log "shutdown" "error flushing database: ~A" e)))
    (shout-log "shutdown" "graceful shutdown complete (~Dms)"
      (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second)))))
  (sb-ext:exit :code 0))

(defun run-api (&key (port 7109) (ops-auth *default-auth*) (admin-auth *default-auth*)
                     tls-cert tls-key)
  ;; GET /info
  (handle-json "/info"
               `((version    . ,*release-version*)
                 (release    . ,*release-name*)
                 (build-date . ,*build-date*)
                 (commit     . ,*build-vcs-id*)))

  ;; GET /state?topic=blah
  (handle-json "/state"
               (with-auth ops-auth
                 (find-state (parameter "topic"))))

   ; GET /states
  (handle-json "/states"
               (with-auth ops-auth
                 (with-lock-held (*states-lock*)
                   (mapcar #'(lambda (a)
                      (state-json (cdr a))) *states*))))

  ;; POST /announce
  (handle-json "/announcements"
               (with-auth ops-auth
                 (if (eq (request-method* *request*) :post)
                   (handler-case
                     (let ((b (json-body)))
                       (notify-announcement
                         (jref b :topic)
                         (make-instance 'event
                           :message     (jref b :message)
                           :link        (jref b :link)
                           :ok          (jref b :ok)
                           :metadata    (jref b :metadata)
                           :occurred-at (or (jref b :occurred-at) (unix-now))))
                       `((ok . "Success!")))
                     (error (e)
                       (shout-log "announcements" "error: ~A" e)
                       (setf (return-code* *reply*) 400)
                       `((error . "invalid request"))))
                   `((oops . "not a POST")
                     (got . ,(request-method *request*))))))

  ;; POST /events
  (handle-json "/events"
               (with-auth ops-auth
                 (if (eq (request-method* *request*) :post)
                   (handler-case
                     (let ((b (json-body)))
                       (with-lock-held (*states-lock*)
                         (set-state
                           (jref b :topic)
                           (make-instance 'event
                             :message     (jref b :message)
                             :link        (jref b :link)
                             :ok          (jref b :ok)
                             :metadata    (jref b :metadata)
                             :occurred-at (or (jref b :occurred-at) (unix-now)))))
                       `((ok . "Success!")))
                     (error (e)
                       (shout-log "events" "error: ~A" e)
                       (setf (return-code* *reply*) 400)
                       `((error . "invalid request"))))
                   `((oops . "not a POST")
                     (got . ,(request-method *request*))))))

  ;; GET/POST /rules
  (handle "/rules"
          (with-auth admin-auth
            (case (request-method* *request*)
              (:post
                (let ((rules-src (raw-post-data :force-text t)))
                  (handler-case
                    (let ((parsed (rules:load/rules rules-src)))
                      (with-lock-held (*rules-lock*)
                        (setf *rules* parsed)
                        (setf *rules-src* rules-src))
                      (setf (content-type* *reply*) "application/json")
                      (format nil "~A~%" (json:encode-json-to-string '((ok . "rules updated")))))
                    (error (e)
                      (shout-log "rules" "error: ~A" e)
                      (setf (return-code* *reply*) 400)
                      (setf (content-type* *reply*) "application/json")
                      (format nil "~A~%" (json:encode-json-to-string
                                           '((error . "invalid request"))))))))
              (:get  (with-lock-held (*rules-lock*) *rules-src*))
              (otherwise
                (setf (return-code *reply*) 400)
                (hunchentoot:abort-request-handler)))))

  (hunchentoot:start
    (if (and tls-cert tls-key)
      (make-instance 'hunchentoot:easy-ssl-acceptor
        :port port
        :ssl-certificate-file tls-cert
        :ssl-privatekey-file tls-key
        :read-timeout 30
        :write-timeout 30
        :message-log-destination nil)
      (make-instance 'hunchentoot:easy-acceptor
        :port port
        :read-timeout 30
        :write-timeout 30
        :message-log-destination nil))))

(defun scan (dbfile)
  (let ((t0 (get-internal-real-time)))
    (handler-case
      (with-lock-held (*states-lock*)
        (when *states-dirty*
          (let ((tw (get-internal-real-time)))
            (write-database dbfile *states*)
            (shout-log "scan" "database written (~Dms)"
              (round (* 1000 (/ (- (get-internal-real-time) tw) internal-time-units-per-second)))))
          (setf *states-dirty* nil))
        (when *expiry*
          (let ((cutoff (- (unix-now) *expiry*)))
            (let ((expired (remove-if-not
                             (lambda (pair)
                               (< (event-occurred-at (last-event (cdr pair))) cutoff))
                             *states*)))
              (when expired
                (setf *states* (remove-if
                                 (lambda (pair)
                                   (< (event-occurred-at (last-event (cdr pair))) cutoff))
                                 *states*))
                (setf *states-dirty* t)
                (shout-log "scan" "expired ~D stale state~:P" (length expired))))))
        (loop for pair in *states*
              do (let ((state (cdr pair)))
                   (when (state-needs-reminder? state)
                     (notify-about-state
                       state (last-event state) "still" (status-of state))))))
      (error (e)
        (shout-log "scan" "error during scan cycle: ~A" e)))
    (shout-log "scan" "cycle complete (~Dms)"
      (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second))))))

(defun run (&key (port *default-port*)
                 (dbfile *default-dbfile*)
                 (expiry *default-expiry*)
                 (ops-auth *default-auth*)
                 (admin-auth *default-auth*)
                 tls-cert tls-key)

  (if (stringp port)
      (setf port (parse-integer port)))

  (when (and tls-cert (not tls-key))
    (error "SHOUT_TLS_CERT is set but SHOUT_TLS_KEY is missing"))
  (when (and tls-key (not tls-cert))
    (error "SHOUT_TLS_KEY is set but SHOUT_TLS_CERT is missing"))

  (setf *dbfile* dbfile)
  (setf *expiry* expiry)

  (shout-log "startup" "registering SIGTERM handler")
  (sb-sys:enable-interrupt sb-posix:sigterm #'handle-sigterm)

  (shout-log "startup" "reading database from ~A" dbfile)
  (let ((t0 (get-internal-real-time)))
    (setf *states* (read-database dbfile))
    (shout-log "startup" "database loaded (~Dms)"
      (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second)))))

  (shout-log "startup" "registering notification plugins")
  (labels ((arg (args name)
             (nth (+ 1 (position name args)) args)))
    (rules:register-plugin
      'rules::slack
      #'(lambda (args)
          (slack:send
            (arg args :text)
            :webhook (arg args :webhook)
            :attachments (list
                           (slack:attach
                             (arg args :attach)
                             :color (arg args :color)))))))

  (shout-log "startup" "binding *:~A~A" port (if (and tls-cert tls-key) " (TLS)" ""))
  (run-api :port port :ops-auth ops-auth :admin-auth admin-auth
           :tls-cert tls-cert :tls-key tls-key)

  (shout-log "startup" "entering upkeep thread main loop")
  (loop until *shutdown* do
    (scan dbfile)
    (sleep 5)))
