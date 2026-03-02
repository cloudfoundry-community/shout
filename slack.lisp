(in-package :slack)

(defvar *default-name* "shout!bot")
(defvar *default-icon* "http://cl.ly/image/3e1h0H3H2s0P/concourse-logo.png")

(defun env (name default)
  (or (sb-unix::posix-getenv name) default))

(defun attach (text &key title color)
  (remove-if #'null
    (list
      (if (null title) nil `(title . ,title))
      (if (null color) nil `(color . ,color))
      `(text . ,text))))

(defun send (text &key (icon     (env "SHOUT_BOTICON" *default-icon*))
                       (username (env "SHOUT_BOTNAME" *default-name*))
                       (webhook  (env "SHOUT_WEBHOOK" ""))
                       (attachments nil))
  (when (equal webhook "")
    (api:shout-log "slack" "no webhook configured, skipping notification")
    (return-from send nil))
  (unless (or (and (>= (length webhook) 8)
                   (string= "https://" webhook :end2 8))
              (and (>= (length webhook) 7)
                   (string= "http://" webhook :end2 7)))
    (api:shout-log "slack" "invalid webhook URL scheme, skipping")
    (return-from send nil))
  (handler-case
    (let ((t0 (get-internal-real-time)))
      (prog1
        (drakma:http-request webhook
                             :method :post
                             :connection-timeout 30
                             :content (json:encode-json-to-string
                                        `((text . ,text)
                                          (username . ,username)
                                          (icon_url . ,icon)
                                          (attachments . ,attachments))))
        (api:shout-log "slack" "notification sent (~Dms)"
          (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second))))))
    (error (e)
      (api:shout-log "slack" "failed to send notification: ~A" e)
      nil)))

