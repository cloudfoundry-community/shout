(in-package :shout)

(defvar *default-daemon-mode*   t)
(defvar *default-pidfile*       #p"/var/run/shout.pid")
(defvar *default-port*          "7109")
(defvar *default-database-file* #p"/var/db/shout.db")
(defvar *default-credentials*   "shout:shout")

(defun env (name &optional default)
  (or (sb-posix:getenv name)
      default))

(defun parse-creds (creds)
  (let ((idx (position #\: creds)))
    (if (null idx)
      (error "Invalid authentication string")
      (cons (subseq creds 0 idx)
            (subseq creds (+ idx 1))))))

(defun shout (&key (daemonize         (env "SHOUT_IT_OUT_LOUD" *default-daemon-mode*))
                   (pidfile           (env "SHOUT_PIDFILE"     *default-pidfile*))
                   (port              (env "SHOUT_PORT"        *default-port*))
                   (database-file     (env "SHOUT_DATABASE"    *default-database-file*))
                   (tls-cert          (env "SHOUT_TLS_CERT"    nil))
                   (tls-key           (env "SHOUT_TLS_KEY"     nil))
                   (ops-credentials   (env "SHOUT_OPS_CREDS"   (env "SHOUT_CREDS" *default-credentials*)))
                   (admin-credentials (env "SHOUT_ADMIN_CREDS" (env "SHOUT_CREDS" *default-credentials*))))

  (if (eq daemonize t)
    (daemon:daemonize :exit-parent t
                      :pidfile pidfile)
    (progn
      (format t " ######  ##     ##  #######  ##     ## ######## #### ~%")
      (format t "##    ## ##     ## ##     ## ##     ##    ##    #### ~%")
      (format t "##       ##     ## ##     ## ##     ##    ##    #### ~%")
      (format t " ######  ######### ##     ## ##     ##    ##     ##  ~%")
      (format t "      ## ##     ## ##     ## ##     ##    ##         ~%")
      (format t "##    ## ##     ## ##     ## ##     ##    ##    #### ~%")
      (format t " ######  ##     ##  #######   #######     ##    #### ~%")
      (api:shout-log "startup" "starting up...")))


  (api:run :port       port
           :dbfile     database-file
           :tls-cert   tls-cert
           :tls-key    tls-key
           :ops-auth   (parse-creds ops-credentials)
           :admin-auth (parse-creds admin-credentials)))
