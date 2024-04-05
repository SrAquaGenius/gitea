// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"strconv"
	"bytes"
	"image"
	"image/jpeg"
	"github.com/nfnt/resize"

	"code.gitea.io/gitea/modules/httpcache"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web/routing"
)

func storageHandler(storageSetting *setting.Storage, prefix string, objStore storage.ObjectStorage) http.HandlerFunc {
	prefix = strings.Trim(prefix, "/")
	funcInfo := routing.GetFuncInfo(storageHandler, prefix)

	if storageSetting.MinioConfig.ServeDirect {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != "GET" && req.Method != "HEAD" {
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}

			if !strings.HasPrefix(req.URL.Path, "/"+prefix+"/") {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
			routing.UpdateFuncInfo(req.Context(), funcInfo)

			rPath := strings.TrimPrefix(req.URL.Path, "/"+prefix+"/")
			rPath = util.PathJoinRelX(rPath)

			u, err := objStore.URL(rPath, path.Base(rPath))
			if err != nil {
				if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
					log.Warn("Unable to find %s %s", prefix, rPath)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				}
				log.Error("Error whilst getting URL for %s %s. Error: %v", prefix, rPath, err)
				http.Error(w, fmt.Sprintf("Error whilst getting URL for %s %s", prefix, rPath), http.StatusInternalServerError)
				return
			}

			http.Redirect(w, req, u.String(), http.StatusTemporaryRedirect)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" && req.Method != "HEAD" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		if !strings.HasPrefix(req.URL.Path, "/"+prefix+"/") {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		routing.UpdateFuncInfo(req.Context(), funcInfo)

		rPath := strings.TrimPrefix(req.URL.Path, "/"+prefix+"/")
		rPath = util.PathJoinRelX(rPath)
		if rPath == "" || rPath == "." {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		fi, err := objStore.Stat(rPath)
		if err != nil {
			if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
				log.Warn("Unable to find %s %s", prefix, rPath)
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
			log.Error("Error whilst opening %s %s. Error: %v", prefix, rPath, err)
			http.Error(w, fmt.Sprintf("Error whilst opening %s %s", prefix, rPath), http.StatusInternalServerError)
			return
		}

		fr, err := objStore.Open(rPath)
		if err != nil {
			log.Error("Error whilst opening %s %s. Error: %v", prefix, rPath, err)
			http.Error(w, fmt.Sprintf("Error whilst opening %s %s", prefix, rPath), http.StatusInternalServerError)
			return
		}
		defer fr.Close()

		// Extract the size value
		query := req.URL.Query()
		avatarStrSize := query.Get("size")

		if avatarStrSize != "" {
			log.Warn("Size value: %s", avatarStrSize)
		} else {
			log.Warn("Size value not found in the URL")
		}

		avatar64Size, err := strconv.ParseUint(avatarStrSize, 10, 64)
		if err != nil {
			log.Error("Couldn't convert to integer")
		}

		avatarSize := uint(avatar64Size)

		// Decode the image
		originalImage, _, err := image.Decode(fr)
		if err != nil {
			log.Error("Error decoding image: %v", err)
		}

		// Resize the image
		newImage := resize.Resize(avatarSize, 0, originalImage, resize.Lanczos3)

		// Encode the resized image as JPEG
		var resizedImageBuf bytes.Buffer
		err = jpeg.Encode(&resizedImageBuf, newImage, nil)
		if err != nil {
			log.Error("Error encoding resized image: %v", err)
		}

		resizedImageReader := bytes.NewReader(resizedImageBuf.Bytes())

		httpcache.ServeContentWithCacheControl(w, req, path.Base(rPath), fi.ModTime(), resizedImageReader)
	})
}
