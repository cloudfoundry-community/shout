(in-package :cl-user)

(defpackage :slack
  (:use :cl
        :drakma
        :cl-json)
  (:export :send
           :attach))

(defpackage :shout
  (:use :cl)
  (:export :shout))

(defpackage :rules
  (:use :cl)
  (:export :register-plugin
           :eval/rules
           :load/rules))

(defpackage :api
  (:use :cl
        :hunchentoot
        :cl-json)
  (:import-from :bordeaux-threads
                :make-lock
                :with-lock-held)
  (:export :run
           :scan
           :shout-log))
