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

(defun build-payload (&key text username icon attachments)
  (remove-if #'null
    (list
      (when text     `(text . ,text))
      (when username `(username . ,username))
      (when icon     `(icon_url . ,icon))
      (when attachments `(attachments . ,attachments)))))

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
                             :content-type "application/json"
                             :content (json:encode-json-to-string
                                        (build-payload :text text
                                                       :username username
                                                       :icon icon
                                                       :attachments attachments)))
        (api:shout-log "slack" "notification sent (~Dms)"
          (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second))))))
    (error (e)
      (api:shout-log "slack" "failed to send notification: ~A" e)
      nil)))

(defun send-api (text &key (token   (env "SHOUT_SLACK_TOKEN" ""))
                            (channel (env "SHOUT_SLACK_CHANNEL" ""))
                            (icon     (env "SHOUT_BOTICON" *default-icon*))
                            (username (env "SHOUT_BOTNAME" *default-name*))
                            (attachments nil))
  (when (equal token "")
    (api:shout-log "slack-app" "no token configured, skipping notification")
    (return-from send-api nil))
  (when (equal channel "")
    (api:shout-log "slack-app" "no channel configured, skipping notification")
    (return-from send-api nil))
  (handler-case
    (let ((t0 (get-internal-real-time)))
      (multiple-value-bind (body status)
        (drakma:http-request (env "SHOUT_SLACK_API_URL" "https://slack.com/api/chat.postMessage")
                             :method :post
                             :content-type "application/json; charset=utf-8"
                             :additional-headers `(("Authorization" . ,(format nil "Bearer ~A" token)))
                             :content (json:encode-json-to-string
                                        (append
                                          `((channel . ,channel))
                                          (build-payload :text text
                                                         :username username
                                                         :icon icon
                                                         :attachments attachments)))
                             :connection-timeout 30)
        (declare (ignore status))
        (let* ((response (json:decode-json-from-string
                           (if (stringp body) body
                               (flexi-streams:octets-to-string body :external-format :utf-8))))
               (ok (cdr (assoc :ok response)))
               (err (cdr (assoc :error response))))
          (unless ok
            (api:shout-log "slack-app" "Slack API error: ~A" (or err "unknown error"))
            (return-from send-api nil))
          (api:shout-log "slack-app" "notification sent (~Dms)"
            (round (* 1000 (/ (- (get-internal-real-time) t0) internal-time-units-per-second)))))))
    (error (e)
      (api:shout-log "slack-app" "failed to send notification: ~A" e)
      nil)))
