import { useState } from 'react';
import './App.css';

const CHUNK_SIZE = 5 * 1024 * 1024;

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

      const uploadPromises = initData.parts.map(async (part) => {
        const start = (part.partNumber - 1) * CHUNK_SIZE;

        const end = Math.min(start + CHUNK_SIZE, file.size);

        const blob = file.slice(start, end);

        console.log(`UPLOAD PART ${part.partNumber}`, {
          start,
          end,
          size: blob.size,
        });

        const uploadRes = await fetch(part.uploadUrl, {
          method: 'PUT',
          body: blob,
        });

        console.log(`PART ${part.partNumber} RESPONSE`, {
          status: uploadRes.status,

          headers: Object.fromEntries(uploadRes.headers.entries()),
        });

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
      });

      const completedParts = await Promise.all(uploadPromises);

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
