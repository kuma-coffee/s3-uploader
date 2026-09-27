import { useState } from 'react';
import './App.css';

const CHUNK_SIZE = 5 * 1024 * 1024;

const MAX_CONCURRENT_UPLOADS = 5;
const MAX_RETRIES = 3;

export default function App() {
  const [file, setFile] = useState(null);
  const [downloadUrl, setDownloadUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState('');

  const upload = async () => {
    if (!file) {
      alert('Pilih file terlebih dahulu');
      return;
    }

    try {
      setLoading(true);
      setProgress(0);
      setDownloadUrl('');

      console.group('🚀 MULTIPART UPLOAD');

      const totalParts = Math.ceil(file.size / CHUNK_SIZE);

      console.log('FILE INFO:', {
        name: file.name,
        size: file.size,
        type: file.type,
        totalParts,
      });

      /*
       * STEP 1
       * INIT MULTIPART
       */

      setStatus('Menginisialisasi upload...');

      const initRes = await fetch('http://localhost:8080/multipart/init', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          fileName: file.name,
          fileSize: file.size,
          totalParts,
        }),
      });

      console.log('INIT HTTP STATUS:', initRes.status);

      const initData = await initRes.json();

      console.log('INIT RESPONSE:', initData);

      /*
       * STEP 2
       * UPLOAD PART
       */

      setStatus(`Uploading ${totalParts} chunks...`);

      let uploadedParts = 0;
      let nextPartIndex = 0;
      let uploadFailed = false;
      const completedParts = [];

      const activeControllers = new Set();

      const uploadPart = async (part) => {
        const start = (part.partNumber - 1) * CHUNK_SIZE;
        const end = Math.min(start + CHUNK_SIZE, file.size);
        const blob = file.slice(start, end);

        let lastError;

        for (let attempt = 1; attempt <= MAX_RETRIES + 1; attempt++) {
          if (uploadFailed) {
            throw new Error('Upload dihentikan karena part lain gagal');
          }

          const controller = new AbortController();

          activeControllers.add(controller);

          try {
            console.log(`UPLOAD PART ${part.partNumber}`, {
              start,
              end,
              size: blob.size,
              attempt,
            });

            const uploadRes = await fetch(part.uploadUrl, {
              method: 'PUT',
              body: blob,
              signal: controller.signal,
            });

            console.log(`PART ${part.partNumber} RESPONSE`, {
              status: uploadRes.status,
              attempt,
              headers: Object.fromEntries(uploadRes.headers.entries()),
            });

            if (!uploadRes.ok) {
              throw new Error(
                `Upload part ${part.partNumber} gagal dengan status ${uploadRes.status}`
              );
            }

            const etag = uploadRes.headers.get('ETag');

            if (!etag) {
              throw new Error(`ETag part ${part.partNumber} tidak ditemukan`);
            }

            uploadedParts++;

            setProgress(Math.round((uploadedParts / totalParts) * 100));

            return {
              partNumber: part.partNumber,
              etag,
            };
          } catch (error) {
            lastError = error;

            if (uploadFailed) {
              throw error;
            }

            if (attempt <= MAX_RETRIES) {
              console.warn(
                `PART ${part.partNumber} gagal. Retry ${attempt}/${MAX_RETRIES}`
              );
            }
          } finally {
            activeControllers.delete(controller);
          }
        }

        uploadFailed = true;

        for (const controller of activeControllers) {
          controller.abort();
        }

        throw lastError;
      };

      const worker = async (workerId) => {
        while (!uploadFailed) {
          const currentIndex = nextPartIndex;

          if (currentIndex >= initData.parts.length) {
            return;
          }

          nextPartIndex++;

          const part = initData.parts[currentIndex];

          console.log(`WORKER ${workerId} menjalankan PART ${part.partNumber}`);

          try {
            const result = await uploadPart(part);

            completedParts.push(result);
          } catch (error) {
            uploadFailed = true;

            for (const controller of activeControllers) {
              controller.abort();
            }

            throw error;
          }
        }
      };

      const workerCount = Math.min(
        MAX_CONCURRENT_UPLOADS,
        initData.parts.length
      );

      const workers = Array.from({ length: workerCount }, (_, index) =>
        worker(index + 1)
      );

      await Promise.all(workers);

      if (uploadFailed) {
        throw new Error('Upload gagal');
      }

      console.log('COMPLETED PARTS:', completedParts);

      /*
       * STEP 3
       * COMPLETE
       */

      setStatus('Menyelesaikan upload...');

      const completeRes = await fetch(
        'http://localhost:8080/multipart/complete',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            uploadId: initData.uploadId,

            objectKey: initData.objectKey,

            parts: completedParts,
          }),
        }
      );

      console.log('COMPLETE HTTP STATUS:', completeRes.status);

      const completeData = await completeRes.json();

      console.log('COMPLETE RESPONSE:', completeData);

      setDownloadUrl(completeData.downloadUrl);

      setStatus('Upload selesai!');

      console.groupEnd();
    } catch (error) {
      console.error('UPLOAD ERROR:', error);

      setStatus('Upload gagal');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className='page'>
      <div className='card'>
        <h1>📦 Multipart Upload</h1>

        <p className='subtitle'>
          Upload file besar menggunakan chunk multipart
        </p>

        <div className='upload-box'>
          <input
            type='file'
            onChange={(e) => {
              if (e.target.files) {
                setFile(e.target.files[0]);
              }
            }}
          />

          {file && (
            <div className='file-info'>
              <b>File:</b>

              <br />

              {file.name}

              <br />

              <small>
                Size: {(file.size / 1024 / 1024).toFixed(2)}
                MB
              </small>
            </div>
          )}
        </div>

        {loading && (
          <div className='progress-container'>
            <div
              className='progress-bar'
              style={{
                width: `${progress}%`,
              }}
            />
          </div>
        )}

        <div className='status'>{status}</div>

        <button onClick={upload} disabled={loading}>
          {loading ? 'Uploading...' : 'Start Upload'}
        </button>

        {downloadUrl && (
          <div className='preview-url'>
            <a href={downloadUrl} target='_blank' rel='noreferrer'>
              {downloadUrl}
            </a>
          </div>
        )}
      </div>
    </div>
  );
}
