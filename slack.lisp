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
  (if (equal webhook "")
    (error "no webhook supplied to slack:send!"))
  (drakma:http-request webhook
                       :method :post
                       :content (json:encode-json-to-string
                                  `((text . ,text)
                                    (username . ,username)
                                    (icon_url . ,icon)
                                    (attachments . ,attachments)))))

(defun send-api (text &key (token   (env "SHOUT_SLACK_TOKEN" ""))
                            (channel (env "SHOUT_SLACK_CHANNEL" ""))
                            (icon     (env "SHOUT_BOTICON" *default-icon*))
                            (username (env "SHOUT_BOTNAME" *default-name*))
                            (attachments nil))
  (when (equal token "")
    (error "no token supplied to slack:send-api! (set SHOUT_SLACK_TOKEN or pass :token)"))
  (when (equal channel "")
    (error "no channel supplied to slack:send-api! (set SHOUT_SLACK_CHANNEL or pass :channel)"))
  (multiple-value-bind (body status)
    (drakma:http-request (env "SHOUT_SLACK_API_URL" "https://slack.com/api/chat.postMessage")
                         :method :post
                         :content-type "application/json; charset=utf-8"
                         :additional-headers `(("Authorization" . ,(format nil "Bearer ~A" token)))
                         :content (json:encode-json-to-string
                                    `((channel . ,channel)
                                      (text . ,text)
                                      (username . ,username)
                                      (icon_url . ,icon)
                                      (attachments . ,attachments)))
                         :connection-timeout 30)
    (declare (ignore status))
    (let* ((response (json:decode-json-from-string
                       (if (stringp body) body
                           (flexi-streams:octets-to-string body :external-format :utf-8))))
           (ok (cdr (assoc :ok response)))
           (err (cdr (assoc :error response))))
      (unless ok
        (error "slack-app API error: ~A" (or err "unknown error"))))))

